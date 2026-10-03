package tgbot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/core"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/service"

	"github.com/op/go-logging"
)

func TestMain(m *testing.M) {
	logger.InitLogger(logging.ERROR)
	m.Run()
}

func TestParseAdminsAcceptsMixedSeparatorsAndDropsJunk(t *testing.T) {
	got := parseAdmins("123, 456;789\n123 abc 0 -5")
	want := []int64{123, 456, 789, -5}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseAdmins = %v, want %v", got, want)
	}
}

func TestParseCommand(t *testing.T) {
	cmd, arg := parseCommand("/Clients@MyBot  alice smith ")
	if cmd != "clients" || arg != "alice smith" {
		t.Fatalf("got %q %q", cmd, arg)
	}
	if cmd, _ := parseCommand("hello"); cmd != "" {
		t.Fatalf("plain text parsed as command %q", cmd)
	}
}

func TestSplitMessageKeepsLinesAndRespectsLimit(t *testing.T) {
	text := strings.Repeat("abcdefghi\n", 10)
	parts := splitMessage(text, 25)
	if len(parts) < 4 {
		t.Fatalf("expected several parts, got %d", len(parts))
	}
	for _, p := range parts {
		if len([]rune(p)) > 25 {
			t.Fatalf("part too long: %d", len([]rune(p)))
		}
	}
	if joined := strings.Join(parts, "\n"); strings.Count(joined, "abcdefghi") != 10 {
		t.Fatal("content lost while splitting")
	}
}

func TestScrubHidesTokenFromErrors(t *testing.T) {
	b := &bot{cfg: botConfig{Token: "123:SECRET"}}
	if got := b.scrub(`Post "https://api.telegram.org/bot123:SECRET/getMe": EOF`); strings.Contains(got, "SECRET") {
		t.Fatalf("token leaked: %s", got)
	}
}

type sent struct {
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text"`
	Method string `json:"-"`
}

func fakeTelegram(t *testing.T) (*bot, func() []sent) {
	t.Helper()
	var mu sync.Mutex
	var messages []sent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		switch method {
		case "sendMessage", "editMessageText", "answerCallbackQuery":
			var m sent
			_ = json.NewDecoder(r.Body).Decode(&m)
			m.Method = method
			mu.Lock()
			messages = append(messages, m)
			mu.Unlock()
		case "sendPhoto", "sendDocument":
			mu.Lock()
			messages = append(messages, sent{Method: method})
			mu.Unlock()
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	t.Cleanup(srv.Close)
	previous := apiBase
	apiBase = srv.URL
	t.Cleanup(func() { apiBase = previous })
	b := newBot(botConfig{Token: "1:abc", Admins: []int64{42}, Lang: "en"})
	return b, func() []sent {
		mu.Lock()
		defer mu.Unlock()
		return append([]sent(nil), messages...)
	}
}

func privateMessage(from int64, text string) update {
	var u update
	raw := `{"update_id":1,"message":{"text":` + mustJSON(text) + `,"chat":{"id":` + itoa(from) + `,"type":"private"},"from":{"id":` + itoa(from) + `}}}`
	_ = json.Unmarshal([]byte(raw), &u)
	return u
}

func mustJSON(s string) string { b, _ := json.Marshal(s); return string(b) }
func itoa(n int64) string      { b, _ := json.Marshal(n); return string(b) }

func TestStrangersOnlyGetTheirIDAndAdminsGetData(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&model.Client{Name: "alice", Enable: true, Volume: 1000, Up: 500, Down: 450, Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`)}).Error; err != nil {
		t.Fatal(err)
	}
	b, got := fakeTelegram(t)
	ctx := context.Background()

	b.handle(ctx, privateMessage(7, "/clients"))
	b.handle(ctx, privateMessage(7, "/id"))
	b.handle(ctx, privateMessage(42, "/clients"))
	b.handle(ctx, privateMessage(42, "/clients alice"))

	msgs := got()
	if len(msgs) != 4 {
		t.Fatalf("messages = %+v", msgs)
	}
	if msgs[0].ChatID != 7 || strings.Contains(msgs[0].Text, "alice") {
		t.Fatalf("stranger was served: %+v", msgs[0])
	}
	if !strings.Contains(msgs[1].Text, "7") {
		t.Fatalf("/id did not echo the ID: %+v", msgs[1])
	}
	if !strings.Contains(msgs[2].Text, "alice") || !strings.Contains(msgs[2].Text, "95%") {
		t.Fatalf("near-limit list = %q", msgs[2].Text)
	}
	if !strings.Contains(msgs[3].Text, "alice") || !strings.Contains(msgs[3].Text, "1000 B") {
		t.Fatalf("client detail = %q", msgs[3].Text)
	}
}

func TestGroupChatsAreIgnored(t *testing.T) {
	b, got := fakeTelegram(t)
	var u update
	_ = json.Unmarshal([]byte(`{"update_id":1,"message":{"text":"/help","chat":{"id":-100,"type":"group"},"from":{"id":42}}}`), &u)
	b.handle(context.Background(), u)
	if len(got()) != 0 {
		t.Fatal("bot answered in a group chat")
	}
}

func callbackFrom(from int64, data string) update {
	var u update
	raw := `{"update_id":2,"callback_query":{"id":"cb1","data":` + mustJSON(data) + `,"from":{"id":` + itoa(from) + `},"message":{"message_id":9,"chat":{"id":` + itoa(from) + `,"type":"private"}}}}`
	_ = json.Unmarshal([]byte(raw), &u)
	return u
}

func texts(msgs []sent, method string) []string {
	var out []string
	for _, m := range msgs {
		if m.Method == method {
			out = append(out, m.Text)
		}
	}
	return out
}

func loadByName(t *testing.T, name string) model.Client {
	t.Helper()
	var c model.Client
	if err := database.GetDB().Where("name = ?", name).First(&c).Error; err != nil {
		t.Fatalf("client %s: %v", name, err)
	}
	return c
}

// The management commands go through ConfigService.Save, the same path as the
// web panel, so this drives a client through its whole life from the bot.
func TestManagementLifecycleThroughCommandsAndButtons(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	// Maintenance keeps Save from trying to start a real core afterwards.
	cs := service.NewConfigService(core.NewCore())
	if err := (&service.SettingService{}).SetMaintenance(true); err != nil {
		t.Fatal(err)
	}
	b, got := fakeTelegram(t)
	b.configService = cs
	ctx := context.Background()

	b.handle(ctx, privateMessage(42, "/add bob 5 30 2"))
	bob := loadByName(t, "bob")
	if !bob.Enable || bob.Volume != 5*gib || bob.LimitIp != 2 || bob.Expiry == 0 || !strings.Contains(string(bob.Config), "xtls-rprx-vision") {
		t.Fatalf("created client = %+v", bob)
	}
	b.handle(ctx, privateMessage(42, "/add bob 5 30"))
	b.handle(ctx, privateMessage(42, "/add bad/name 5 30"))
	var count int64
	database.GetDB().Model(model.Client{}).Count(&count)
	if count != 1 {
		t.Fatalf("duplicate or invalid names were created: %d clients", count)
	}

	id := itoa(int64(bob.Id))
	b.handle(ctx, callbackFrom(42, "c:tog:"+id))
	if loadByName(t, "bob").Enable {
		t.Fatal("toggle did not disable the client")
	}
	b.handle(ctx, privateMessage(42, "/enable bob"))
	b.handle(ctx, callbackFrom(42, "c:gb10:"+id))
	b.handle(ctx, callbackFrom(42, "c:d30:"+id))
	after := loadByName(t, "bob")
	if !after.Enable || after.Volume != 15*gib || after.Expiry < bob.Expiry+29*86400 {
		t.Fatalf("after edits = %+v", after)
	}

	b.handle(ctx, privateMessage(42, "/volume bob 2"))
	b.handle(ctx, privateMessage(42, "/limitip bob 0"))
	b.handle(ctx, privateMessage(42, "/expiry bob 0"))
	after = loadByName(t, "bob")
	if after.Volume != 2*gib || after.LimitIp != 0 || after.Expiry != 0 {
		t.Fatalf("after /volume /limitip /expiry = %+v", after)
	}
	// An unlimited client cannot have time added to it.
	b.handle(ctx, callbackFrom(42, "c:d30:"+id))
	if loadByName(t, "bob").Expiry != 0 {
		t.Fatal("added days to an unlimited client")
	}

	// Reset needs a confirmation press and keeps lifetime totals.
	database.GetDB().Model(model.Client{}).Where("id = ?", bob.Id).Updates(map[string]interface{}{"up": 100, "down": 200})
	b.handle(ctx, callbackFrom(42, "c:rst:"+id))
	if loadByName(t, "bob").Up != 100 {
		t.Fatal("reset happened before it was confirmed")
	}
	b.handle(ctx, callbackFrom(42, "c:rsty:"+id))
	after = loadByName(t, "bob")
	if after.Up != 0 || after.Down != 0 || after.TotalUp != 100 || after.TotalDown != 200 {
		t.Fatalf("after reset = %+v", after)
	}

	// Binding lets the owner look at their own client, and survives panel edits.
	b.handle(ctx, privateMessage(42, "/bind bob 99"))
	if loadByName(t, "bob").TgId != 99 {
		t.Fatal("bind did not store the Telegram ID")
	}
	b.handle(ctx, privateMessage(42, "/volume bob 3"))
	if loadByName(t, "bob").TgId != 99 {
		t.Fatal("an edit erased the Telegram binding")
	}
	before := len(got())
	b.handle(ctx, privateMessage(99, "/usage"))
	b.handle(ctx, privateMessage(99, "/sub"))
	b.handle(ctx, privateMessage(99, "/add eve 1 1"))
	b.handle(ctx, callbackFrom(99, "c:dely:"+id))
	msgs := got()[before:]
	if len(msgs) < 4 || !strings.Contains(msgs[0].Text, "bob") || strings.Contains(msgs[0].Text, "xtls") {
		t.Fatalf("user view = %+v", msgs)
	}
	if loadByName(t, "bob").Id != bob.Id {
		t.Fatal("a bound user deleted their client")
	}
	database.GetDB().Model(model.Client{}).Count(&count)
	if count != 1 {
		t.Fatal("a bound user created a client")
	}
	var photos int
	for _, m := range msgs {
		if m.Method == "sendPhoto" {
			photos++
		}
	}
	if photos != 1 {
		t.Fatalf("QR photos sent = %d", photos)
	}

	b.handle(ctx, privateMessage(42, "/unbind bob"))
	if loadByName(t, "bob").TgId != 0 {
		t.Fatal("unbind failed")
	}
	b.handle(ctx, callbackFrom(42, "c:dely:"+id))
	database.GetDB().Model(model.Client{}).Count(&count)
	if count != 0 {
		t.Fatal("delete did not remove the client")
	}
	if len(texts(got(), "editMessageText")) == 0 {
		t.Fatal("button presses did not edit the message")
	}
}

func TestStrangerButtonPressesAreRefused(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	c := model.Client{Name: "carol", Enable: true, Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`)}
	database.GetDB().Create(&c)
	b, got := fakeTelegram(t)
	b.handle(context.Background(), callbackFrom(7, "c:tog:"+itoa(int64(c.Id))))
	b.handle(context.Background(), callbackFrom(7, "m:restarty"))
	if !loadByName(t, "carol").Enable {
		t.Fatal("a stranger toggled a client")
	}
	for _, m := range got() {
		if m.Method != "answerCallbackQuery" {
			t.Fatalf("stranger got %s", m.Method)
		}
	}
}

func TestNewClientConfigCoversEveryProtocol(t *testing.T) {
	var cfg map[string]map[string]interface{}
	if err := json.Unmarshal(newClientConfig("zed"), &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg) != 15 || cfg["vless"]["name"] != "zed" || cfg["mixed"]["username"] != "zed" {
		t.Fatalf("config = %v", cfg)
	}
	if id, _ := cfg["vmess"]["uuid"].(string); len(id) != 36 || id[14] != '4' {
		t.Fatalf("uuid = %q", id)
	}
	if !clientNameRe.MatchString("a.b-c_d@e") || clientNameRe.MatchString("a b") {
		t.Fatal("name validation")
	}
}

func testBot(t *testing.T) (*bot, func() []sent) {
	t.Helper()
	if err := database.InitDB(filepath.Join(t.TempDir(), "bot.db")); err != nil {
		t.Fatal(err)
	}
	// Saves finish their restart/fan-out work in goroutines; let them end
	// before the next test swaps the global database.
	t.Cleanup(func() { time.Sleep(300 * time.Millisecond) })
	cs := service.NewConfigService(core.NewCore())
	if err := (&service.SettingService{}).SetMaintenance(true); err != nil {
		t.Fatal(err)
	}
	// Settings rows are created lazily; the config row must exist to be saved.
	if _, err := (&service.SettingService{}).GetAllSetting(); err != nil {
		t.Fatal(err)
	}
	b, got := fakeTelegram(t)
	b.configService = cs
	return b, got
}

func lastEdit(t *testing.T, got func() []sent) string {
	t.Helper()
	edits := texts(got(), "editMessageText")
	if len(edits) == 0 {
		t.Fatal("no message was edited")
	}
	return edits[len(edits)-1]
}

// Every button of the main menu has to open a screen without error.
func TestEveryMenuScreenOpens(t *testing.T) {
	b, got := testBot(t)
	ctx := context.Background()
	database.GetDB().Create(&model.Client{Name: "zoe", Enable: true, Inbounds: json.RawMessage(`[]`), Links: json.RawMessage(`[]`)})
	b.handle(ctx, privateMessage(42, "/menu"))
	for _, row := range b.mainMenu() {
		for _, btn := range row {
			// These two really stop and start the core.
			if btn.Data == "m:maint" || btn.Data == "m:restart" {
				continue
			}
			before := len(got())
			b.handle(ctx, callbackFrom(42, btn.Data))
			msgs := got()[before:]
			if len(msgs) == 0 {
				t.Fatalf("%s: no response", btn.Data)
			}
			for _, m := range msgs {
				if strings.Contains(m.Text, "❌") {
					t.Fatalf("%s failed: %s", btn.Data, m.Text)
				}
			}
		}
	}
	for _, data := range []string{"c:ls:e:0", "c:ls:d:0", "c:ls:n:0", "c:ls:x:0", "c:ls:o:0", "c:clean", "s:ls", "s:g:p", "s:g:s", "s:g:m", "g:rs:ls:0", "g:dr:ls:0", "t:d7", "t:d30", "m:logs:err", "g:log:lv:debug"} {
		before := len(got())
		b.handle(ctx, callbackFrom(42, data))
		for _, m := range got()[before:] {
			if strings.Contains(m.Text, "❌") {
				t.Fatalf("%s failed: %s", data, m.Text)
			}
		}
	}
}

func TestObjectsCreateEditAndDeleteThroughJSON(t *testing.T) {
	b, got := testBot(t)
	ctx := context.Background()
	b.handle(ctx, callbackFrom(42, "o:out:n:0"))
	b.handle(ctx, privateMessage(42, "```json\n"+templateJSON("out", "direct")+"\n```"))
	var ob model.Outbound
	if err := database.GetDB().Where("tag = ?", "direct-out").First(&ob).Error; err != nil {
		t.Fatalf("outbound was not created: %v (%s)", err, lastEdit(t, got))
	}
	b.handle(ctx, callbackFrom(42, "o:out:e:"+itoa(int64(ob.Id))))
	b.handle(ctx, privateMessage(42, `{"type":"direct","tag":"renamed-out"}`))
	var count int64
	database.GetDB().Model(model.Outbound{}).Where("tag = ?", "renamed-out").Count(&count)
	if count != 1 {
		t.Fatalf("edit did not apply: %s", lastEdit(t, got))
	}
	// A bad answer keeps the prompt open instead of dropping it.
	b.handle(ctx, callbackFrom(42, "o:out:e:"+itoa(int64(ob.Id))))
	b.handle(ctx, privateMessage(42, `{not json`))
	if b.pend.get(42) == nil {
		t.Fatal("prompt was dropped after a bad answer")
	}
	b.handle(ctx, callbackFrom(42, "x:cancel"))
	if b.pend.get(42) != nil {
		t.Fatal("cancel did not clear the prompt")
	}
	b.handle(ctx, callbackFrom(42, "o:out:d:"+itoa(int64(ob.Id))))
	b.handle(ctx, callbackFrom(42, "o:out:dy:"+itoa(int64(ob.Id))))
	database.GetDB().Model(model.Outbound{}).Where("tag = ?", "renamed-out").Count(&count)
	if count != 0 {
		t.Fatal("outbound was not deleted")
	}

	// Inbounds: create, change the port, attach a client, delete.
	b.handle(ctx, callbackFrom(42, "o:in:n:0"))
	b.handle(ctx, privateMessage(42, `{"type":"mixed","tag":"mixed-in","listen":"::","listen_port":2080,"tls_id":0}`))
	var in model.Inbound
	if err := database.GetDB().Where("tag = ?", "mixed-in").First(&in).Error; err != nil {
		t.Fatalf("inbound was not created: %s", lastEdit(t, got))
	}
	b.handle(ctx, callbackFrom(42, "o:in:port:"+itoa(int64(in.Id))))
	b.handle(ctx, privateMessage(42, "2090"))
	database.GetDB().First(&in, in.Id)
	if !strings.Contains(string(in.Options), "2090") {
		t.Fatalf("port not changed: %s", in.Options)
	}
	b.handle(ctx, privateMessage(42, "/add dan 1 1"))
	dan := loadByName(t, "dan")
	if !strings.Contains(string(dan.Inbounds), itoa(int64(in.Id))) {
		t.Fatalf("new client was not attached to the inbound: %s", dan.Inbounds)
	}
	b.handle(ctx, callbackFrom(42, "c:ti:"+itoa(int64(dan.Id))+":"+itoa(int64(in.Id))))
	if got := loadByName(t, "dan").Inbounds; strings.Contains(string(got), itoa(int64(in.Id))) {
		t.Fatalf("toggle did not detach the inbound: %s", got)
	}
	b.handle(ctx, callbackFrom(42, "o:in:dy:"+itoa(int64(in.Id))))
	database.GetDB().Model(model.Inbound{}).Where("tag = ?", "mixed-in").Count(&count)
	if count != 0 {
		t.Fatal("inbound was not deleted")
	}
}

func TestClientWizardAndFieldPrompts(t *testing.T) {
	b, _ := testBot(t)
	ctx := context.Background()
	b.handle(ctx, callbackFrom(42, "c:new"))
	b.handle(ctx, privateMessage(42, "wiz1"))
	b.handle(ctx, callbackFrom(42, "w:grp:-"))
	b.handle(ctx, callbackFrom(42, "w:vol:20"))
	b.handle(ctx, callbackFrom(42, "w:days:30"))
	b.handle(ctx, callbackFrom(42, "w:ip:2"))
	c := loadByName(t, "wiz1")
	if c.Volume != 20*gib || c.LimitIp != 2 || c.Expiry == 0 {
		t.Fatalf("wizard client = %+v", c)
	}
	id := itoa(int64(c.Id))
	for field, answer := range map[string]string{"desc": "vip", "grp": "team", "tg": "555", "vol": "7"} {
		b.handle(ctx, callbackFrom(42, "c:ask:"+field+":"+id))
		b.handle(ctx, privateMessage(42, answer))
	}
	c = loadByName(t, "wiz1")
	if c.Desc != "vip" || c.Group != "team" || c.TgId != 555 || c.Volume != 7*gib {
		t.Fatalf("after prompts = %+v", c)
	}
	b.handle(ctx, callbackFrom(42, "c:ask:name:"+id))
	b.handle(ctx, privateMessage(42, "wiz2"))
	if loadByName(t, "wiz2").Id != c.Id {
		t.Fatal("rename failed")
	}
	// Depleted clients are removed by the cleanup.
	database.GetDB().Model(model.Client{}).Where("id = ?", c.Id).Updates(map[string]interface{}{"up": 8 * gib})
	b.handle(ctx, callbackFrom(42, "c:cleany"))
	var count int64
	database.GetDB().Model(model.Client{}).Count(&count)
	if count != 0 {
		t.Fatal("cleanup did not delete the depleted client")
	}
	b.handle(ctx, privateMessage(42, "/addbulk bulk 3 1 1"))
	database.GetDB().Model(model.Client{}).Count(&count)
	if count != 3 {
		t.Fatalf("bulk created %d clients", count)
	}
}

func TestConfigRulesAndSettings(t *testing.T) {
	b, _ := testBot(t)
	ctx := context.Background()
	rules := func() []interface{} {
		m, err := b.loadConfigMap()
		if err != nil {
			t.Fatal(err)
		}
		return getPath(m, []string{"route", "rules"})
	}
	base := len(rules())
	b.handle(ctx, callbackFrom(42, "g:rl:n:0"))
	b.handle(ctx, privateMessage(42, `{"action":"route","outbound":"direct","domain_suffix":["example.com"]}`))
	if len(rules()) != base+1 {
		t.Fatalf("rule not added: %d -> %d", base, len(rules()))
	}
	last := strconv.Itoa(base)
	b.handle(ctx, callbackFrom(42, "g:rl:up:"+last))
	moved := rules()[base-1].(map[string]interface{})
	if scalar(moved["outbound"]) != "direct" {
		t.Fatalf("rule not moved up: %v", moved)
	}
	b.handle(ctx, callbackFrom(42, "g:rl:dy:"+strconv.Itoa(base-1)))
	if len(rules()) != base {
		t.Fatal("rule not deleted")
	}
	b.handle(ctx, callbackFrom(42, "g:log:lv:warn"))
	m, _ := b.loadConfigMap()
	if scalar(asMap(m["log"])["level"]) != "warn" {
		t.Fatalf("log level = %v", m["log"])
	}

	b.handle(ctx, callbackFrom(42, "s:tg:subEncode"))
	all, _ := (&service.SettingService{}).GetAllSetting()
	first := (*all)["subEncode"]
	b.handle(ctx, callbackFrom(42, "s:tg:subEncode"))
	all, _ = (&service.SettingService{}).GetAllSetting()
	if (*all)["subEncode"] == first {
		t.Fatal("boolean setting did not toggle")
	}
	b.handle(ctx, callbackFrom(42, "s:ask:subDomain"))
	b.handle(ctx, privateMessage(42, "sub.example.com"))
	all, _ = (&service.SettingService{}).GetAllSetting()
	if (*all)["subDomain"] != "sub.example.com" {
		t.Fatalf("subDomain = %q", (*all)["subDomain"])
	}
}

func TestClientPanelParityEditConfigLinksJSONAndBulk(t *testing.T) {
	b, got := testBot(t)
	ctx := context.Background()
	press := func(data string) { b.handle(ctx, callbackFrom(42, data)) }
	say := func(text string) { b.handle(ctx, privateMessage(42, text)) }

	// New client from JSON with the friendly template fields.
	press("c:newj")
	say(`{"name":"jc1","desc":"d","remark":"r","group":"g1","volumeGB":5,"days":10,"limitIp":2,"autoReset":true,"resetDays":7}`)
	c := loadByName(t, "jc1")
	if c.Volume != 5*gib || c.Expiry == 0 || c.LimitIp != 2 || !c.AutoReset || c.ResetDays != 7 || c.Remark != "r" || c.Group != "g1" {
		t.Fatalf("json client = %+v", c)
	}
	id := itoa(int64(c.Id))

	// Every sub-screen opens.
	for _, d := range []string{"c:view:" + id, "c:edit:" + id, "c:cfg:" + id, "c:xl:" + id, "c:bulk", "c:bk:fa", "c:bk:g0"} {
		press(d)
		if lastEdit(t, got) == "" {
			t.Fatalf("%s opened an empty screen", d)
		}
	}

	// Remark, exact expiry date and reset days.
	press("c:ask:rem:" + id)
	say("hello")
	press("c:ask:exp:" + id)
	say("2031-05-06 07:08")
	press("c:ask:rd:" + id)
	say("3")
	c = loadByName(t, "jc1")
	if c.Remark != "hello" || c.ResetDays != 3 || time.Unix(c.Expiry, 0).In(time.Local).Format("2006-01-02 15:04") != "2031-05-06 07:08" {
		t.Fatalf("after edit prompts = %+v", c)
	}

	// Delay start / auto reset switches.
	press("c:ar:" + id)
	if loadByName(t, "jc1").AutoReset {
		t.Fatal("auto reset should be off")
	}
	press("c:dly:" + id)
	if c = loadByName(t, "jc1"); !c.DelayStart || c.Expiry != 0 {
		t.Fatalf("delay start = %+v", c)
	}
	press("c:dly:" + id)

	// Credentials: regenerate one protocol, then replace via JSON.
	before := parseClientCfg(loadByName(t, "jc1").Config)
	press("c:cfgp:" + id + ":" + itoa(int64(indexOf(before.keys(), "vless"))))
	after := parseClientCfg(loadByName(t, "jc1").Config)
	if before["vless"]["uuid"] == after["vless"]["uuid"] || before["trojan"]["password"] != after["trojan"]["password"] {
		t.Fatal("regenerating one protocol changed the wrong credentials")
	}
	press("c:cfga:" + id)
	if parseClientCfg(loadByName(t, "jc1").Config)["trojan"]["password"] == before["trojan"]["password"] {
		t.Fatal("regenerate all did not change trojan")
	}
	press("c:ask:cfgj:" + id)
	say(`{"trojan":{"name":"jc1","password":"fixed"}}`)
	if parseClientCfg(loadByName(t, "jc1").Config)["trojan"]["password"] != "fixed" {
		t.Fatal("config JSON was not applied")
	}

	// External and subscription links.
	press("c:ask:xl:" + id)
	say("vless://abc@example.com:443#x")
	press("c:ask:sl:" + id)
	say("https://example.com/sub/x")
	press("c:ask:xl:" + id)
	say("not a link")
	var links []map[string]interface{}
	_ = json.Unmarshal(loadByName(t, "jc1").Links, &links)
	if len(links) != 2 {
		t.Fatalf("links = %v", links)
	}
	press("c:xld:" + id + ":0")
	_ = json.Unmarshal(loadByName(t, "jc1").Links, &links)
	if len(links) != 1 || links[0]["type"] != "sub" {
		t.Fatalf("links after delete = %v", links)
	}

	// Whole-client JSON edit keeps protected fields.
	press("c:json:" + id)
	say(`{"desc":"via json","limitIp":9,"up":123456,"tgId":77}`)
	if c = loadByName(t, "jc1"); c.Desc != "via json" || c.LimitIp != 9 || c.Up != 0 || c.TgId != 0 {
		t.Fatalf("json edit = %+v", c)
	}

	// Bulk edit on the group "g1" and on everyone.
	press("c:bk:g0:days")
	say("5")
	press("c:bk:fa:vol")
	say("2")
	press("c:bk:fa:ip")
	say("4")
	if c = loadByName(t, "jc1"); c.Volume != 7*gib || c.LimitIp != 4 {
		t.Fatalf("bulk edit = %+v", c)
	}
	press("c:bk:fa:dis:y")
	if loadByName(t, "jc1").Enable {
		t.Fatal("bulk disable failed")
	}
	press("c:bk:fa:rst:y")
	press("c:bk:fa:del")
	press("c:bk:fa:del:y")
	var count int64
	database.GetDB().Model(model.Client{}).Count(&count)
	if count != 0 {
		t.Fatalf("bulk delete left %d clients", count)
	}
}

func indexOf(list []string, v string) int {
	for i, s := range list {
		if s == v {
			return i
		}
	}
	return -1
}

func TestClientsAreListedInCreationOrderAndWizardSuggestsAName(t *testing.T) {
	b, _ := testBot(t)
	ctx := context.Background()
	// Two legacy rows without a creation time, then three dated ones whose
	// names are deliberately not alphabetical.
	for _, row := range []struct {
		name string
		at   int64
	}{{"leg1", 0}, {"leg2", 0}, {"xq1", 1000}, {"bq2", 1001}, {"aq3", 1002}} {
		c := model.Client{Enable: true, Name: row.name, Config: newClientConfig(row.name), Inbounds: json.RawMessage("[]"), Links: json.RawMessage("[]"), CreatedAt: row.at}
		if err := database.GetDB().Create(&c).Error; err != nil {
			t.Fatal(err)
		}
	}
	names := func(list []model.Client) string {
		var out []string
		for _, c := range list {
			out = append(out, c.Name)
		}
		return strings.Join(out, ",")
	}
	// Default: the order the clients were created in, oldest first.
	if got := names(b.filterClients("a", loadClients())); got != "leg1,leg2,xq1,bq2,aq3" {
		t.Fatalf("default order = %s", got)
	}
	// The toggle button flips it, the choice is saved, and search follows it.
	b.handle(ctx, callbackFrom(42, "c:sort:a"))
	if got := names(b.filterClients("a", loadClients())); got != "aq3,bq2,xq1,leg2,leg1" {
		t.Fatalf("newest first = %s", got)
	}
	text, _ := b.clientsView("q")
	if !(strings.Index(text, "aq3") < strings.Index(text, "bq2") && strings.Index(text, "bq2") < strings.Index(text, "xq1")) {
		t.Fatalf("search results are not newest first:\n%s", text)
	}
	screen, _ := b.clientsScreen("a", 0)
	if !strings.Contains(screen, "· 📅 1970-01-01") {
		t.Fatalf("list lines carry no creation date:\n%s", screen)
	}
	b.handle(ctx, callbackFrom(42, "c:sort:a"))
	if got := names(b.filterClients("a", loadClients())); got != "leg1,leg2,xq1,bq2,aq3" {
		t.Fatalf("order after toggling back = %s", got)
	}

	// Use the suggested name as is.
	b.handle(ctx, callbackFrom(42, "c:new"))
	p := b.pend.get(42)
	if p == nil || len(p.data["rnd"]) != 8 {
		t.Fatalf("no suggestion: %+v", p)
	}
	first := p.data["rnd"]
	b.handle(ctx, callbackFrom(42, "w:name:#new"))
	if b.pend.get(42).data["rnd"] == first {
		t.Fatal("reroll kept the same name")
	}
	suggested := b.pend.get(42).data["rnd"]
	b.handle(ctx, callbackFrom(42, "w:name:#ok"))
	b.handle(ctx, callbackFrom(42, "w:grp:-"))
	b.handle(ctx, callbackFrom(42, "w:vol:1"))
	b.handle(ctx, callbackFrom(42, "w:days:0"))
	b.handle(ctx, callbackFrom(42, "w:ip:0"))
	loadByName(t, suggested)

	// Append to the suggestion with "+".
	b.handle(ctx, callbackFrom(42, "c:new"))
	suggested = b.pend.get(42).data["rnd"]
	b.handle(ctx, privateMessage(42, "+_ali"))
	b.handle(ctx, callbackFrom(42, "w:grp:-"))
	b.handle(ctx, callbackFrom(42, "w:vol:1"))
	b.handle(ctx, callbackFrom(42, "w:days:0"))
	b.handle(ctx, callbackFrom(42, "w:ip:0"))
	loadByName(t, suggested+"_ali")
}

// callbackData flattens a keyboard into the callback data of its buttons.
func callbackData(kb [][]button) []string {
	var out []string
	for _, row := range kb {
		for _, btn := range row {
			out = append(out, btn.Data)
		}
	}
	return out
}

func hasData(kb [][]button, want string) bool { return indexOf(callbackData(kb), want) >= 0 }

func TestClientGroupsAreShownFilteredAndChosenInTheBot(t *testing.T) {
	b, got := testBot(t)
	ctx := context.Background()
	press := func(data string) { b.handle(ctx, callbackFrom(42, data)) }
	say := func(text string) { b.handle(ctx, privateMessage(42, text)) }
	groupOf := func(name string) string { return loadByName(t, name).Group }

	// Groups made in the web panel: two hand-made ones, one client without a
	// group and one the cluster pushed (reserved group).
	for _, row := range []struct{ name, group string }{{"a1", "Team"}, {"a2", "vip"}, {"a3", ""}, {"n1", service.ClusterGroup}} {
		c := model.Client{Enable: true, Name: row.name, Group: row.group, Config: newClientConfig(row.name), Inbounds: json.RawMessage("[]"), Links: json.RawMessage("[]")}
		if err := database.GetDB().Create(&c).Error; err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.Join(b.clientGroups(), ","); got != "Team,vip" {
		t.Fatalf("groups = %q (sorted, reserved one left out)", got)
	}

	// The list shows each client's group; the reserved one is hidden.
	line := func(name string) string { return b.clientLine(loadByName(t, name), time.Now()) }
	if !strings.Contains(line("a1"), "🏷 Team") || !strings.Contains(line("a2"), "🏷 vip") {
		t.Fatalf("lines carry no group: %q / %q", line("a1"), line("a2"))
	}
	if strings.Contains(line("a3"), "🏷") || strings.Contains(line("n1"), "🏷") {
		t.Fatalf("no-group / cluster clients show a group: %q / %q", line("a3"), line("n1"))
	}
	if text, _ := b.clientsScreen("a", 0); !strings.Contains(text, "🏷 Team") || !strings.Contains(text, "🏷 vip") {
		t.Fatalf("client list shows no groups:\n%s", text)
	}

	// Search matches the group name too.
	if text, _ := b.clientsView("VIP"); !strings.Contains(text, "a2") || strings.Contains(text, "a1") {
		t.Fatalf("search by group:\n%s", text)
	}

	// The Groups screen, and the filters behind it.
	text, kb := b.groupsScreen()
	if !strings.Contains(text, "Team") || !strings.Contains(text, "vip") || strings.Contains(text, service.ClusterGroup) {
		t.Fatalf("groups screen:\n%s", text)
	}
	for _, want := range []string{"c:ls:g0:0", "c:ls:g1:0", "c:ls:u:0"} {
		if !hasData(kb, want) {
			t.Fatalf("groups screen lacks %s: %v", want, callbackData(kb))
		}
	}
	if _, listKB := b.clientsScreen("a", 0); !hasData(listKB, "c:grps") {
		t.Fatal("client list has no Groups button")
	}
	press("c:grps")
	if !strings.Contains(lastEdit(t, got), "Groups") {
		t.Fatal("c:grps did not open the groups screen")
	}
	names := func(list []model.Client) string {
		var out []string
		for _, c := range list {
			out = append(out, c.Name)
		}
		return strings.Join(out, ",")
	}
	for filter, want := range map[string]string{"g0": "a1", "g1": "a2", "u": "a3", "g9": "", "gx": ""} {
		if got := names(b.filterClients(filter, loadClients())); got != want {
			t.Fatalf("filter %s = %q, want %q", filter, got, want)
		}
	}
	press("c:ls:g0:0")
	if screen := lastEdit(t, got); !strings.Contains(screen, "🏷 Team") || !strings.Contains(screen, "a1") || strings.Contains(screen, "a2") {
		t.Fatalf("group filter screen:\n%s", screen)
	}
	press("c:ls:u:0")
	if screen := lastEdit(t, got); !strings.Contains(screen, "a3") || strings.Contains(screen, "a1") {
		t.Fatalf("no-group filter screen:\n%s", screen)
	}

	// New-client wizard: the group step offers the existing groups ...
	press("c:new")
	say("w1")
	p := b.pend.get(42)
	if p == nil || p.key != "grp" {
		t.Fatalf("wizard did not ask for a group: %+v", p)
	}
	prompt, choices := b.wizardPrompt(p)
	if !strings.Contains(prompt, "2/5") {
		t.Fatalf("group prompt: %s", prompt)
	}
	for _, want := range []string{"w:grp:0", "w:grp:1", "w:grp:-"} {
		if !hasData(choices, want) {
			t.Fatalf("group choices lack %s: %v", want, callbackData(choices))
		}
	}
	// ... and a pressed button picks one of them.
	press("w:grp:1")
	press("w:vol:1")
	press("w:days:0")
	press("w:ip:0")
	if groupOf("w1") != "vip" {
		t.Fatalf("w1 group = %q", groupOf("w1"))
	}
	// A typed name creates a new group,
	press("c:new")
	say("w2")
	say("  Fresh start ")
	press("w:vol:1")
	press("w:days:0")
	press("w:ip:0")
	if groupOf("w2") != "Fresh start" {
		t.Fatalf("w2 group = %q", groupOf("w2"))
	}
	if got := strings.Join(b.clientGroups(), ","); got != "Fresh start,Team,vip" {
		t.Fatalf("groups after creating one = %q", got)
	}
	// a name that differs from an existing group only by case reuses it,
	press("c:new")
	say("w3")
	say("TEAM")
	press("w:vol:1")
	press("w:days:0")
	press("w:ip:0")
	if groupOf("w3") != "Team" {
		t.Fatalf("w3 group = %q (case-insensitive reuse)", groupOf("w3"))
	}
	// the reserved cluster name is refused and the step stays open,
	press("c:new")
	say("w4")
	say("@CLUSTER")
	if p = b.pend.get(42); p == nil || p.key != "grp" {
		t.Fatalf("reserved group name was accepted: %+v", p)
	}
	// and "no group" leaves it empty.
	press("w:grp:-")
	press("w:vol:1")
	press("w:days:0")
	press("w:ip:0")
	if groupOf("w4") != "" {
		t.Fatalf("w4 group = %q", groupOf("w4"))
	}
	// A group button pressed on a later step is stale, not applied.
	press("c:new")
	say("w5")
	press("w:grp:-")
	press("w:grp:0")
	if p = b.pend.get(42); p == nil || p.key != "vol" || p.data["grp"] != "" {
		t.Fatalf("stale group button changed the wizard: %+v", p)
	}
	press("x:cancel")

	// Editing a client: the chooser, a typed new group, and clearing.
	id := itoa(int64(loadByName(t, "a3").Id))
	press("c:ask:grp:" + id)
	if p = b.pend.get(42); p == nil || p.kind != "cl.grp" {
		t.Fatalf("group editor not pending: %+v", p)
	}
	press("c:sg:" + id + ":0")
	if groupOf("a3") != "Fresh start" {
		t.Fatalf("chooser did not set the group: %q", groupOf("a3"))
	}
	if b.pend.get(42) != nil {
		t.Fatal("the pending prompt survived a button choice")
	}
	press("c:sg:" + id + ":-")
	if groupOf("a3") != "" {
		t.Fatalf("no-group button left %q", groupOf("a3"))
	}
	press("c:sg:" + id + ":99")
	if groupOf("a3") != "" {
		t.Fatalf("a stale index changed the group: %q", groupOf("a3"))
	}
	press("c:ask:grp:" + id)
	say("Brand new")
	if groupOf("a3") != "Brand new" {
		t.Fatalf("typed group = %q", groupOf("a3"))
	}
	press("c:ask:grp:" + id)
	say("@cluster")
	if groupOf("a3") != "Brand new" {
		t.Fatalf("reserved group was accepted: %q", groupOf("a3"))
	}
	say("-")
	if groupOf("a3") != "" {
		t.Fatalf("typed dash left %q", groupOf("a3"))
	}

	// /addbulk takes an optional group after the IP limit.
	say("/addbulk bk 2 1 1 0 Two words")
	if groupOf("bk1") != "Two words" || groupOf("bk2") != "Two words" {
		t.Fatalf("bulk groups = %q / %q", groupOf("bk1"), groupOf("bk2"))
	}
	say("/addbulk cc 1 1 1 0 @cluster")
	var count int64
	database.GetDB().Model(model.Client{}).Where("name = ?", "cc1").Count(&count)
	if count != 0 {
		t.Fatal("bulk create accepted the reserved group")
	}
	say("/addbulk pl 1 1 1")
	if groupOf("pl1") != "" {
		t.Fatalf("bulk without a group = %q", groupOf("pl1"))
	}

	// JSON create normalises the group the same way and refuses the reserved one.
	press("c:newj")
	say(`{"name":"j1","group":"vIp"}`)
	if groupOf("j1") != "vip" {
		t.Fatalf("json group = %q", groupOf("j1"))
	}
	press("c:newj")
	say(`{"name":"j2","group":"@cluster"}`)
	database.GetDB().Model(model.Client{}).Where("name = ?", "j2").Count(&count)
	if count != 0 {
		t.Fatal("JSON create accepted the reserved group")
	}
	press("c:json:" + id)
	say(`{"group":"@cluster"}`)
	if groupOf("a3") != "" {
		t.Fatalf("JSON edit accepted the reserved group: %q", groupOf("a3"))
	}
}

// The panel's "add all inbounds to all clients" button is in the bot too, under
// bulk edit: it asks first, adds what is missing and says how many clients got it.
func TestAttachAllInboundsToAllClientsFromTheBot(t *testing.T) {
	b, got := testBot(t)
	ctx := context.Background()
	press := func(data string) { b.handle(ctx, callbackFrom(42, data)) }

	var inboundIDs []uint
	for _, in := range []model.Inbound{
		{Tag: "in-vless", Type: "vless", Options: json.RawMessage(`{"listen":"::","listen_port":443}`)},
		{Tag: "in-trojan", Type: "trojan", Options: json.RawMessage(`{"listen":"::","listen_port":8443}`)},
		{Tag: "in-direct", Type: "direct", Options: json.RawMessage(`{"listen_port":53}`)},
	} {
		in.Addrs, in.OutJson = json.RawMessage("[]"), json.RawMessage("{}")
		if err := database.GetDB().Create(&in).Error; err != nil {
			t.Fatal(err)
		}
		if in.Type != "direct" {
			inboundIDs = append(inboundIDs, in.Id)
		}
	}
	for _, name := range []string{"c1", "c2", "c3"} {
		c := model.Client{Enable: true, Name: name, Config: newClientConfig(name), Inbounds: json.RawMessage("[]"), Links: json.RawMessage("[]")}
		if err := database.GetDB().Create(&c).Error; err != nil {
			t.Fatal(err)
		}
	}

	if _, kb := b.bulkScopeScreen(); !hasData(kb, "c:att") {
		t.Fatalf("bulk edit has no attach-all button: %v", callbackData(kb))
	}
	press("c:att")
	screen := lastEdit(t, got)
	if !strings.Contains(screen, "3 clients lack some of the 2 inbounds") {
		t.Fatalf("confirmation screen:\n%s", screen)
	}
	if _, kb := b.attachAllScreen(); !hasData(kb, "c:att:y") {
		t.Fatalf("confirmation has no confirm button: %v", callbackData(kb))
	}
	// Nothing happens before the confirmation.
	if got := clientInboundIDs(loadByName(t, "c1")); len(got) != 0 {
		t.Fatalf("clients changed before the confirmation: %v", got)
	}

	press("c:att:y")
	if screen = lastEdit(t, got); !strings.Contains(screen, "Added the missing inbounds to 3 clients") {
		t.Fatalf("result screen:\n%s", screen)
	}
	for _, name := range []string{"c1", "c2", "c3"} {
		if ids := clientInboundIDs(loadByName(t, name)); !reflect.DeepEqual(ids, inboundIDs) {
			t.Fatalf("%s inbounds = %v, want %v", name, ids, inboundIDs)
		}
	}
	press("c:att")
	if screen = lastEdit(t, got); !strings.Contains(screen, "Every client already has every inbound") {
		t.Fatalf("screen once everything is attached:\n%s", screen)
	}
}
