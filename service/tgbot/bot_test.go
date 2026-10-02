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

func TestClientsAreListedNewestFirstAndWizardSuggestsAName(t *testing.T) {
	b, _ := testBot(t)
	ctx := context.Background()
	for i, n := range []string{"zzz", "aaa", "mmm"} {
		c := model.Client{Enable: true, Name: n, Config: newClientConfig(n), Inbounds: json.RawMessage("[]"), Links: json.RawMessage("[]"), CreatedAt: int64(1000 + i)}
		if err := database.GetDB().Create(&c).Error; err != nil {
			t.Fatal(err)
		}
	}
	list := b.filterClients("a", loadClients())
	if len(list) != 3 || list[0].Name != "mmm" || list[1].Name != "aaa" || list[2].Name != "zzz" {
		t.Fatalf("order = %v", list)
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
	b.handle(ctx, callbackFrom(42, "w:vol:1"))
	b.handle(ctx, callbackFrom(42, "w:days:0"))
	b.handle(ctx, callbackFrom(42, "w:ip:0"))
	loadByName(t, suggested)

	// Append to the suggestion with "+".
	b.handle(ctx, callbackFrom(42, "c:new"))
	suggested = b.pend.get(42).data["rnd"]
	b.handle(ctx, privateMessage(42, "+_ali"))
	b.handle(ctx, callbackFrom(42, "w:vol:1"))
	b.handle(ctx, callbackFrom(42, "w:days:0"))
	b.handle(ctx, callbackFrom(42, "w:ip:0"))
	loadByName(t, suggested+"_ali")
}
