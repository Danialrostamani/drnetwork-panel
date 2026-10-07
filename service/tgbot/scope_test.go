package tgbot

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

const (
	fullAdmin   = int64(42)
	salesAdmin  = int64(77)
	salesGroup  = "Sales"
	otherGroup  = "Other"
	depletedVol = int64(100)
)

func seedClient(t *testing.T, name, group string, volume, used int64) model.Client {
	t.Helper()
	c := model.Client{Enable: true, Name: name, Group: group, Volume: volume, Up: used, Config: newClientConfig(name), Inbounds: json.RawMessage("[]"), Links: json.RawMessage("[]")}
	if err := database.GetDB().Create(&c).Error; err != nil {
		t.Fatal(err)
	}
	return c
}

type scopeEnv struct {
	t   *testing.T
	b   *bot
	got func() []sent
	id  map[string]uint
}

// newScopeEnv has admin 42 with full access and admin 77 limited to the
// "Sales" group, over clients in that group (one spelled in lower case, which
// is still the same group), another group, no group and the reserved cluster
// group.
func newScopeEnv(t *testing.T) *scopeEnv {
	t.Helper()
	b, got := testBot(t)
	useAccess(b, botConfig{Admins: []int64{fullAdmin, salesAdmin}, Scopes: map[int64]string{salesAdmin: salesGroup}})
	e := &scopeEnv{t: t, b: b, got: got, id: map[string]uint{}}
	for _, row := range []struct {
		name, group string
		volume      int64
		used        int64
	}{
		{"s1", salesGroup, 1 << 30, 0},
		{"s2", "sales", 0, 0},
		{"sd", salesGroup, depletedVol, depletedVol},
		{"o1", otherGroup, 1 << 30, 0},
		{"od", otherGroup, depletedVol, depletedVol},
		{"n1", "", 0, 0},
		{"cl1", service.ClusterGroup, 0, 0},
	} {
		e.id[row.name] = seedClient(t, row.name, row.group, row.volume, row.used).Id
	}
	return e
}

func (e *scopeEnv) say(from int64, text string) {
	e.t.Helper()
	e.b.handle(context.Background(), privateMessage(from, text))
}

func (e *scopeEnv) press(from int64, data string) {
	e.t.Helper()
	e.b.handle(context.Background(), callbackFrom(from, data))
}

func (e *scopeEnv) cid(name string) string { return itoa(int64(e.id[name])) }

// since returns what the bot sent after the first n messages.
func (e *scopeEnv) since(n int) []sent { return e.got()[n:] }

func (e *scopeEnv) client(name string) model.Client { return loadByName(e.t, name) }

func (e *scopeEnv) exists(name string) bool {
	var n int64
	database.GetDB().Model(model.Client{}).Where("name = ?", name).Count(&n)
	return n > 0
}

func clientNames(list []model.Client) string {
	out := make([]string, 0, len(list))
	for _, c := range list {
		out = append(out, c.Name)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

// keyboardOf is the inline keyboard of the last message that had one.
func keyboardOf(msgs []sent) [][]button {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Markup != nil && len(msgs[i].Markup.Keyboard) > 0 {
			return msgs[i].Markup.Keyboard
		}
	}
	return nil
}

func lastText(msgs []sent, methods ...string) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		for _, m := range methods {
			if msgs[i].Method == m {
				return msgs[i].Text
			}
		}
	}
	return ""
}

func TestParseScopesAndConfig(t *testing.T) {
	got, locked := parseScopes("77=Sales\n 88 = Team A \r\nnoequals\n=NoID\n77=Duplicate\n0=Zero\nabc=Text\n-5=Negative\n# a comment\n\n15=Good\n15=\n")
	want := map[int64]string{77: "Sales", 88: "Team A", -5: "Negative", 15: "Good"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("parseScopes = %v, want %v", got, want)
	}
	if len(locked) != 0 {
		t.Fatalf("clean lines locked %v", locked)
	}
	if empty, locked := parseScopes(""); len(empty) != 0 || len(locked) != 0 {
		t.Fatal("empty setting gave scopes")
	}

	// A line that names an ID but cannot be used must not leave that person
	// unlimited: the ID is locked out instead. A good line for the same ID wins.
	got, locked = parseScopes("99=\n12 Sales\n13\n14 x=Sales\n55=@CLUSTER\n66=" + strings.Repeat("x", 65) + "\n77=Sales\n77=")
	if fmt.Sprint(got) != "map[77:Sales]" || fmt.Sprint(locked) != "[12 13 14 55 66 99]" {
		t.Fatalf("scopes = %v, locked = %v", got, locked)
	}

	// A line in the scope list is enough to make someone an administrator,
	// so a forgotten entry in the admin list cannot leave a limit off; an ID
	// whose line is unusable is an administrator with no access at all, even
	// when it is in the admin list.
	acc := buildAccess(0, []int64{42, 77, 99}, map[int64]string{88: "B", 77: "A", 12: "C"}, nil, []int64{99})
	if fmt.Sprint(acc.order) != "[42 77 99 12 88]" {
		t.Fatalf("administrators = %v", acc.order)
	}
	if r := acc.roleOf(77); r.group != "A" || r.sections != nil {
		t.Fatalf("role of 77 = %+v", r)
	}
	if r := acc.roleOf(99); r.sections == nil || len(r.sections) != 0 || r.group != "" {
		t.Fatalf("a locked admin has access: %+v", r)
	}
	if r := acc.roleOf(42); !r.full() {
		t.Fatalf("role of 42 = %+v", r)
	}

	// The fingerprint says when the bot has to be restarted: not when only
	// who may do what changed, which the running bot takes over live.
	a := botConfig{Token: "1:a", Admins: []int64{1}, Scopes: map[int64]string{1: "A", 2: "B"}}
	b := botConfig{Token: "1:a", Admins: []int64{1}, Scopes: map[int64]string{2: "B", 1: "A"}}
	if a.fingerprint() != b.fingerprint() || a.accessKey() != b.accessKey() {
		t.Fatal("a key depends on map order")
	}
	b.Scopes[2] = "C"
	if a.fingerprint() != b.fingerprint() {
		t.Fatal("a changed group limit would restart the bot")
	}
	if a.accessKey() == b.accessKey() {
		t.Fatal("a changed scope did not change the access key, so the bot would keep the old limit")
	}
	b = botConfig{Token: "1:a", Admins: []int64{1}, Scopes: map[int64]string{2: "B", 1: "A"}, Locked: []int64{7}}
	if a.accessKey() == b.accessKey() {
		t.Fatal("a newly locked admin did not change the access key")
	}
	b = a
	b.Token = "2:b"
	if a.fingerprint() == b.fingerprint() {
		t.Fatal("a new token did not change the fingerprint")
	}
}

func TestScopeSettingReachesTheBotConfig(t *testing.T) {
	testBot(t)
	save := func(values map[string]string) {
		t.Helper()
		raw, _ := json.Marshal(values)
		if err := (&service.SettingService{}).Save(database.GetDB(), raw); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := loadConfig()
	if err != nil || len(cfg.Scopes) != 0 {
		t.Fatalf("default config: %+v %v", cfg, err)
	}
	save(map[string]string{"tgBotAdmins": "42", "tgBotScopes": "77=Sales\n88=Team A"})
	cfg, err = loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(cfg.Admins) != "[42]" || cfg.Scopes[77] != "Sales" || cfg.Scopes[88] != "Team A" || fmt.Sprint(cfg.access().order) != "[42 77 88]" {
		t.Fatalf("config = %+v", cfg)
	}
	before, beforeAccess := cfg.fingerprint(), cfg.accessKey()
	save(map[string]string{"tgBotScopes": "77=Support"})
	cfg, _ = loadConfig()
	if cfg.fingerprint() != before || cfg.accessKey() == beforeAccess || cfg.Scopes[77] != "Support" || len(cfg.Scopes) != 1 {
		t.Fatalf("edited scopes not picked up: %+v", cfg)
	}

	// A typo in a limit must not turn an administrator into a full one: the
	// ID has no access at all until the line is fixed.
	save(map[string]string{"tgBotAdmins": "42, 77", "tgBotScopes": "77 Sales"})
	cfg, _ = loadConfig()
	if r := cfg.access().roleOf(77); !cfg.access().isMember(77) || r.group != "" || r.sections == nil || len(r.sections) != 0 || fmt.Sprint(cfg.Locked) != "[77]" || len(cfg.Scopes) != 0 {
		t.Fatalf("a malformed limit line left admin 77 with access: %+v", cfg)
	}
	b, got := fakeTelegram(t)
	useAccess(b, cfg)
	b.handle(context.Background(), privateMessage(77, "/clients"))
	if len(got()) != 1 || lastText(got(), "sendMessage") != b.t("noAccess") {
		t.Fatalf("a locked admin got more than a refusal: %v", got())
	}
	b.handle(context.Background(), callbackFrom(77, "c:ls:a:0"))
	if lastText(got(), "answerCallbackQuery") != b.t("noAccess") {
		t.Fatalf("a locked admin pressed a button: %v", got())
	}
	b.handle(context.Background(), callbackFrom(42, "a:ls"))
	if txt := lastText(got(), "editMessageText", "sendMessage"); !strings.Contains(txt, "77") || !strings.Contains(txt, "⛔") {
		t.Fatalf("the admins screen does not list the locked admin: %q", txt)
	}
}

func TestScopedAdminSeesOnlyTheirGroup(t *testing.T) {
	e := newScopeEnv(t)
	limited := e.b.as(salesAdmin)
	if limited.scope != salesGroup || e.b.as(fullAdmin).scope != "" || e.b.as(999).scope != "" {
		t.Fatal("as() did not scope the right administrator")
	}
	if got := clientNames(limited.loadClients()); got != "s1,s2,sd" {
		t.Fatalf("limited clients = %s", got)
	}
	if got := clientNames(e.b.as(fullAdmin).loadClients()); got != "cl1,n1,o1,od,s1,s2,sd" {
		t.Fatalf("full clients = %s", got)
	}

	screen, _ := limited.clientsScreen("a", 0)
	for _, want := range []string{"s1", "s2", "sd", "(3/3)"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("limited list lacks %q:\n%s", want, screen)
		}
	}
	for _, bad := range []string{"o1", "od", "n1", "cl1", "Other"} {
		if strings.Contains(screen, bad) {
			t.Fatalf("limited list shows %q:\n%s", bad, screen)
		}
	}

	// Commands: the near-limit list, a search and the group search.
	n := len(e.got())
	e.say(salesAdmin, "/clients")
	if text := lastText(e.since(n), "sendMessage"); !strings.Contains(text, "sd") || strings.Contains(text, "od") {
		t.Fatalf("/clients:\n%s", text)
	}
	for _, query := range []string{"o1", "Other", "n1", "cl1", "@cluster"} {
		n = len(e.got())
		e.say(salesAdmin, "/clients "+query)
		if text := lastText(e.since(n), "sendMessage"); text != "No clients found." {
			t.Fatalf("/clients %s showed %q", query, text)
		}
	}
	n = len(e.got())
	e.say(salesAdmin, "/clients sales")
	if text := lastText(e.since(n), "sendMessage"); !strings.Contains(text, "s1") || !strings.Contains(text, "s2") || strings.Contains(text, "o1") {
		t.Fatalf("/clients sales:\n%s", text)
	}
	n = len(e.got())
	e.say(fullAdmin, "/clients other")
	if text := lastText(e.since(n), "sendMessage"); !strings.Contains(text, "o1") {
		t.Fatalf("the full administrator lost sight of a group:\n%s", text)
	}

	// Opening a client of another group by id is "not found", not a card.
	for _, name := range []string{"o1", "n1", "cl1"} {
		n = len(e.got())
		e.press(salesAdmin, "c:view:"+e.cid(name))
		msgs := e.since(n)
		if text := lastText(msgs, "editMessageText"); text != "" {
			t.Fatalf("c:view:%s opened a card:\n%s", name, text)
		}
		if answer := lastText(msgs, "answerCallbackQuery"); answer != "No client with that name." {
			t.Fatalf("c:view:%s answered %q", name, answer)
		}
	}
	n = len(e.got())
	e.press(salesAdmin, "c:view:"+e.cid("s1"))
	if text := lastText(e.since(n), "editMessageText"); !strings.Contains(text, "s1") {
		t.Fatalf("own client did not open:\n%s", text)
	}

	// Online users, the IP list and the home page are the group's too.
	previous := allOnlineUsers
	allOnlineUsers = func() []string { return []string{"cl1", "n1", "o1", "s1"} }
	t.Cleanup(func() { allOnlineUsers = previous })
	if got := strings.Join(limited.onlineUsers(), ","); got != "s1" {
		t.Fatalf("limited online = %s", got)
	}
	if got := strings.Join(e.b.as(fullAdmin).onlineUsers(), ","); got != "cl1,n1,o1,s1" {
		t.Fatalf("full online = %s", got)
	}
	if text := limited.onlineText(); !strings.Contains(text, "s1") || strings.Contains(text, "o1") || strings.Contains(text, "n1") {
		t.Fatalf("online text:\n%s", text)
	}
	if text, _ := limited.clientsScreen("o", 0); !strings.Contains(text, "s1") || strings.Contains(text, "o1") {
		t.Fatalf("online filter:\n%s", text)
	}
	if text := limited.ipsText("o1"); text != "No clients found." {
		t.Fatalf("/ips of another group's client: %q", text)
	}
	home := limited.homeText()
	for _, want := range []string{"Sales", "Clients: 3", "online: 1"} {
		if !strings.Contains(home, want) {
			t.Fatalf("scoped home lacks %q:\n%s", want, home)
		}
	}
	for _, bad := range []string{"CPU", "RAM", "sing-box", "IPv4", "Nodes"} {
		if strings.Contains(home, bad) {
			t.Fatalf("scoped home leaks %q:\n%s", bad, home)
		}
	}
	if text := limited.trafficText(); strings.Contains(text, "od") || strings.Contains(text, "o1") {
		t.Fatalf("traffic text:\n%s", text)
	}
}

func TestScopedAdminCannotChangeOtherGroupsClients(t *testing.T) {
	e := newScopeEnv(t)
	foreign := []string{"o1", "od", "n1", "cl1"}
	before := map[string]model.Client{}
	for _, name := range foreign {
		before[name] = e.client(name)
	}

	for _, name := range foreign {
		for _, cmd := range []string{"enable", "disable", "reset", "del", "delete", "volume %s 5", "expiry %s 5", "limitip %s 3", "bind %s 555", "unbind", "sub", "ips"} {
			text := "/" + cmd + " " + name
			if strings.Contains(cmd, "%s") {
				text = "/" + fmt.Sprintf(cmd, name)
			}
			n := len(e.got())
			e.say(salesAdmin, text)
			msgs := e.since(n)
			if len(msgs) != 1 || (msgs[0].Text != "No client with that name." && msgs[0].Text != "No clients found.") {
				t.Fatalf("%s answered %+v", text, msgs)
			}
		}
		for _, cb := range []string{
			"view", "tog", "rst", "rsty", "gb10", "gb50", "d30", "edit", "cfg", "cfga", "xl", "inb", "sub", "lnk", "ips", "kick", "json", "del", "dely", "dly", "ar",
			"ask:vol", "ask:name", "ask:tg", "ask:json", "cfgp", "xld", "ti",
		} {
			data := "c:" + cb + ":" + e.cid(name)
			switch cb {
			case "ask:vol", "ask:name", "ask:tg", "ask:json":
				data = "c:ask:" + strings.TrimPrefix(cb, "ask:") + ":" + e.cid(name)
			case "cfgp", "xld", "ti":
				data += ":1"
			}
			n := len(e.got())
			e.press(salesAdmin, data)
			msgs := e.since(n)
			for _, m := range msgs {
				if m.Method != "answerCallbackQuery" {
					t.Fatalf("%s produced %s: %q", data, m.Method, m.Text)
				}
			}
			if e.b.pend.get(salesAdmin) != nil {
				t.Fatalf("%s armed a prompt for another group's client", data)
			}
		}
	}
	for _, name := range foreign {
		after := e.client(name)
		if fmt.Sprintf("%+v", before[name]) != fmt.Sprintf("%+v", after) {
			t.Fatalf("%s changed:\nbefore %+v\nafter  %+v", name, before[name], after)
		}
	}

	// The write guard holds even if a screen forgot to check: every kind of
	// save that touches another group, or moves a client out of the group.
	limited := e.b.as(salesAdmin)
	mine, theirs := e.client("s1"), e.client("o1")
	moved := mine
	moved.Group = otherGroup
	ungrouped := mine
	ungrouped.Group = ""
	hijack := theirs
	hijack.Group = salesGroup // pulling another group's client into mine is a change to it, too
	for name, call := range map[string]func() error{
		"edit foreign":    func() error { return limited.save("edit", &theirs) },
		"edit move out":   func() error { return limited.save("edit", &moved) },
		"edit ungroup":    func() error { return limited.save("edit", &ungrouped) },
		"edit take over":  func() error { return limited.save("edit", &hijack) },
		"edit without id": func() error { c := mine; c.Id = 0; return limited.save("edit", &c) },
		"del foreign":     func() error { return limited.save("del", theirs.Id) },
		"delbulk mixed":   func() error { return limited.save("delbulk", []uint{mine.Id, theirs.Id}) },
		"delbulk none":    func() error { return limited.save("delbulk", []uint{}) },
		"editbulk mixed":  func() error { return limited.save("editbulk", []model.Client{mine, theirs}) },
		"editbulk move":   func() error { return limited.save("editbulk", []model.Client{moved}) },
		"new other group": func() error { return limited.save("new", model.Client{Name: "x1", Group: otherGroup}) },
		"new no group":    func() error { return limited.save("new", model.Client{Name: "x2"}) },
		"new cluster":     func() error { return limited.save("new", model.Client{Name: "x3", Group: service.ClusterGroup}) },
		"new with id":     func() error { return limited.save("new", model.Client{Id: theirs.Id, Name: "x4", Group: salesGroup}) },
		"addbulk mixed": func() error {
			return limited.save("addbulk", []model.Client{{Name: "x5", Group: salesGroup}, {Name: "x6"}})
		},
		"attach all":          func() error { return limited.save("attachall", struct{}{}) },
		"unknown action":      func() error { return limited.save("whatever", mine) },
		"wrong payload":       func() error { return limited.save("edit", "s1") },
		"bind foreign":        func() error { return limited.bindClient(theirs.Id, 5) },
		"bind missing client": func() error { return limited.bindClient(99999, 5) },
	} {
		if err := call(); err == nil {
			t.Errorf("%s was allowed", name)
		}
	}
	if !e.exists("s1") || !e.exists("o1") || e.exists("x1") || e.exists("x2") || e.exists("x3") || e.exists("x4") || e.exists("x5") || e.exists("x6") {
		t.Fatal("a refused save still changed the clients")
	}
	if g := e.client("s1").Group; g != salesGroup {
		t.Fatalf("s1 group = %q", g)
	}
	if o := e.client("o1"); o.Group != otherGroup || o.TgId != 0 {
		t.Fatalf("o1 = %+v", o)
	}

	// And their own clients work as before.
	e.say(salesAdmin, "/volume s1 5")
	if v := e.client("s1").Volume; v != 5*gib {
		t.Fatalf("s1 volume = %d", v)
	}
	e.say(salesAdmin, "/disable s2")
	if e.client("s2").Enable {
		t.Fatal("s2 is still enabled")
	}
	e.press(salesAdmin, "c:tog:"+e.cid("s2"))
	if !e.client("s2").Enable {
		t.Fatal("the toggle button did not enable s2")
	}
	e.say(salesAdmin, "/bind s1 555")
	if tg := e.client("s1").TgId; tg != 555 {
		t.Fatalf("s1 tgId = %d", tg)
	}
	e.say(salesAdmin, "/limitip s1 2")
	if l := e.client("s1").LimitIp; l != 2 {
		t.Fatalf("s1 limit = %d", l)
	}
	// The JSON editor may keep the group's own spelling but not leave the group.
	e.press(salesAdmin, "c:json:"+e.cid("s1"))
	e.say(salesAdmin, `{"desc":"hello","group":"SALES"}`)
	if c := e.client("s1"); c.Desc != "hello" || c.Group != "SALES" {
		t.Fatalf("in-group JSON edit: %+v", c)
	}
	for _, body := range []string{`{"group":"Other"}`, `{"group":""}`, `{"group":"@cluster"}`, `{"name":"o1"}`} {
		e.press(salesAdmin, "c:json:"+e.cid("s1"))
		e.say(salesAdmin, body)
		if c := e.client("s1"); c.Group != "SALES" || c.Name != "s1" {
			t.Fatalf("JSON %s changed s1 to %+v", body, c)
		}
	}
}

func TestScopedAdminCannotReachTheServer(t *testing.T) {
	e := newScopeEnv(t)
	denied := "This is not available to group-limited admins."
	for _, cmd := range []string{"inbounds", "nodes", "settings", "changes", "backup", "logs", "logs debug", "sync", "restart", "maintenance off", "maintenance on", "stats", "traffic", "bogus"} {
		n := len(e.got())
		e.say(salesAdmin, "/"+cmd)
		msgs := e.since(n)
		if len(msgs) != 1 || msgs[0].Method != "sendMessage" || msgs[0].Text != denied {
			t.Fatalf("/%s answered %+v", cmd, msgs)
		}
	}
	if on, _ := (&service.SettingService{}).GetMaintenance(); !on {
		t.Fatal("maintenance mode was switched off by a limited admin")
	}

	for _, data := range []string{
		"o:in:ls:0", "o:nd:ls:0", "o:out:ls:0", "o:ep:ls:0", "g:rl:ls:0", "g:dn:ls:0", "g:basics:ls:0", "s:ls", "s:g:p", "s:tg:maintenance", "s:ask:webPort", "t:d1", "t:d7", "a:ls", "x:ls",
		"m:backup", "m:restart", "m:restarty", "m:maint", "m:logs:info", "m:sync", "m:status", "m:nodes", "m:inbounds", "m:traffic", "m:prest", "m:prestY",
		"c:att", "c:att:y", "c:grps", "c:sg:" + "1:-", "c:ask:grp:1", "bogus:x",
	} {
		n := len(e.got())
		e.press(salesAdmin, data)
		msgs := e.since(n)
		if len(msgs) != 1 || msgs[0].Method != "answerCallbackQuery" || msgs[0].Text != denied {
			t.Fatalf("%s answered %+v", data, msgs)
		}
	}
	if on, _ := (&service.SettingService{}).GetMaintenance(); !on {
		t.Fatal("maintenance mode was switched off by a limited admin")
	}

	// A prompt for something that is not a client field is dropped, not applied.
	e.b.pend.set(salesAdmin, &pending{kind: "set", key: "webPort", data: map[string]string{}})
	e.say(salesAdmin, "9999")
	if port, _ := (&service.SettingService{}).GetPort(); port == 9999 {
		t.Fatal("a limited admin changed a panel setting")
	}
	if e.b.pend.get(salesAdmin) != nil {
		t.Fatal("the foreign prompt stayed armed")
	}
	e.b.pend.set(salesAdmin, &pending{kind: "obj.new", key: "in", data: map[string]string{}})
	e.say(salesAdmin, `{"type":"mixed","tag":"evil","listen":"::","listen_port":9999}`)
	var inbounds int64
	database.GetDB().Model(model.Inbound{}).Count(&inbounds)
	if inbounds != 0 {
		t.Fatal("a limited admin created an inbound")
	}

	// The full administrator is not affected by any of this.
	n := len(e.got())
	e.say(fullAdmin, "/inbounds")
	if text := lastText(e.since(n), "sendMessage"); text == denied || text == "" {
		t.Fatalf("/inbounds for the full admin: %q", text)
	}
	n = len(e.got())
	e.press(fullAdmin, "a:ls")
	text := lastText(e.since(n), "editMessageText")
	if !strings.Contains(text, "Bot admins") || !strings.Contains(text, "77") || !strings.Contains(text, "Sales") {
		t.Fatalf("the admins screen does not show the limit:\n%s", text)
	}
	if !hasData(e.b.as(fullAdmin).mainMenu(), "o:in:ls:0") || !hasData(e.b.as(fullAdmin).mainMenu(), "m:backup") {
		t.Fatal("the full menu lost buttons")
	}
	if hasData(e.b.as(salesAdmin).mainMenu(), "o:in:ls:0") {
		t.Fatal("the limited menu has an inbounds button")
	}
}

// Every button the bot shows a limited administrator has to work for them: no
// dead buttons, and nothing that is only there for full administrators.
func TestScopedScreensOnlyOfferAllowedButtons(t *testing.T) {
	e := newScopeEnv(t)
	limited := e.b.as(salesAdmin)
	c := e.client("s1")
	full, _ := limited.fullClient(c.Id)
	check := func(name string, kb [][]button) {
		t.Helper()
		if len(kb) == 0 {
			t.Fatalf("%s has no buttons", name)
		}
		for _, d := range callbackData(kb) {
			if d == "noop" {
				continue
			}
			if !scopedCallbackAllowed(strings.Split(d, ":")) {
				t.Errorf("%s offers %q, which a limited admin cannot press", name, d)
			}
		}
	}
	check("menu", limited.mainMenu())
	check("home", limited.homeKeyboard())
	_, kb := limited.clientsScreen("a", 0)
	check("clients", kb)
	if hasData(kb, "c:grps") {
		t.Error("the clients list offers the groups screen")
	}
	_, kb = limited.card(c)
	check("card", kb)
	_, kb = limited.clientEditScreen(*full)
	check("edit", kb)
	for _, d := range callbackData(kb) {
		if strings.HasPrefix(d, "c:ask:grp:") {
			t.Error("the edit screen offers a group change")
		}
	}
	_, kb = limited.clientConfigScreen(*full)
	check("config", kb)
	_, kb = limited.clientLinksScreen(*full)
	check("links", kb)
	_, kb = limited.clientInboundsScreen(*full)
	check("inbounds of a client", kb)
	_, kb = limited.bulkScopeScreen()
	check("bulk scope", kb)
	if hasData(kb, "c:att") {
		t.Error("bulk edit offers 'add all inbounds to all clients'")
	}
	_, kb = limited.bulkActionsScreen("fa", "")
	check("bulk actions", kb)
	for _, step := range []string{"name", "vol", "days", "ip"} {
		_, kb = limited.wizardPrompt(&pending{kind: "wiz", key: step, data: map[string]string{}})
		check("wizard "+step, kb)
	}
	_, kb = limited.clientsView("s1")
	check("search", kb)
	if !hasData(limited.scopedMenu(), "c:new") {
		t.Error("the limited menu cannot create a client")
	}
}

func TestScopedAdminCreatesOnlyInTheirGroup(t *testing.T) {
	e := newScopeEnv(t)
	group := func(name string) string { return e.client(name).Group }

	// The wizard has no group step and puts the client in the group, using the
	// spelling the group's clients already have.
	e.press(salesAdmin, "c:new")
	e.say(salesAdmin, "w1")
	p := e.b.pend.get(salesAdmin)
	if p == nil || p.key != "vol" || p.data["grp"] != salesGroup {
		t.Fatalf("wizard after the name: %+v", p)
	}
	if prompt, _ := e.b.as(salesAdmin).wizardPrompt(p); !strings.Contains(prompt, "2/4") {
		t.Fatalf("limited wizard numbering:\n%s", prompt)
	}
	n := len(e.got())
	e.press(salesAdmin, "w:grp:-")
	if p = e.b.pend.get(salesAdmin); p == nil || p.key != "vol" || p.data["grp"] != salesGroup {
		t.Fatalf("a stale group button changed the wizard: %+v", p)
	}
	if answer := lastText(e.since(n), "answerCallbackQuery"); !strings.Contains(answer, "expired") {
		t.Fatalf("stale button answered %q", answer)
	}
	e.press(salesAdmin, "w:vol:5")
	e.press(salesAdmin, "w:days:0")
	e.press(salesAdmin, "w:ip:0")
	if group("w1") != salesGroup || e.client("w1").Volume != 5*gib {
		t.Fatalf("w1 = %+v", e.client("w1"))
	}
	// A name used in another group is taken: names are unique panel-wide.
	e.press(salesAdmin, "c:new")
	e.say(salesAdmin, "o1")
	if p = e.b.pend.get(salesAdmin); p == nil || p.key != "name" {
		t.Fatalf("a name from another group was accepted: %+v", p)
	}
	e.press(salesAdmin, "x:cancel")

	e.say(salesAdmin, "/add a1 1 0")
	e.say(salesAdmin, "/add o1 1 0")
	e.say(salesAdmin, "/addbulk b 2 1 1")
	e.say(salesAdmin, "/addbulk c 1 1 1 0 Other")
	e.say(salesAdmin, "/addbulk d 1 1 1 0 SALES")
	e.say(salesAdmin, "/addbulk f 1 1 1 0 @cluster")
	for name, want := range map[string]string{"a1": salesGroup, "b1": salesGroup, "b2": salesGroup, "d1": salesGroup} {
		if group(name) != want {
			t.Errorf("%s group = %q, want %q", name, group(name), want)
		}
	}
	for _, name := range []string{"c1", "f1"} {
		if e.exists(name) {
			t.Errorf("%s was created in a group the admin does not have", name)
		}
	}
	var olds int64
	database.GetDB().Model(model.Client{}).Where("name = ?", "o1").Count(&olds)
	if olds != 1 {
		t.Fatalf("/add o1 made a duplicate name (%d)", olds)
	}

	// JSON: the template carries the group, an empty group means the group,
	// and any other group is refused.
	n = len(e.got())
	e.press(salesAdmin, "c:newj")
	if text := html.UnescapeString(lastText(e.since(n), "editMessageText")); !strings.Contains(text, `"group": "Sales"`) {
		t.Fatalf("JSON template:\n%s", text)
	}
	e.say(salesAdmin, `{"name":"j1"}`)
	e.press(salesAdmin, "c:newj")
	e.say(salesAdmin, `{"name":"j2","group":"Other"}`)
	e.press(salesAdmin, "c:newj")
	e.say(salesAdmin, `{"name":"j3","group":""}`)
	e.press(salesAdmin, "c:newj")
	e.say(salesAdmin, `{"name":"j4","group":"sAlEs"}`)
	if group("j1") != salesGroup || group("j3") != salesGroup || group("j4") != salesGroup || e.exists("j2") {
		t.Fatalf("JSON create: j1=%q j3=%q j4=%q j2 exists=%v", group("j1"), group("j3"), group("j4"), e.exists("j2"))
	}

	// The group cannot be edited away.
	n = len(e.got())
	e.press(salesAdmin, "c:ask:grp:"+e.cid("s1"))
	e.press(salesAdmin, "c:sg:"+e.cid("s1")+":-")
	e.press(salesAdmin, "c:sg:"+e.cid("s1")+":#new")
	e.say(salesAdmin, "Elsewhere")
	if group("s1") != salesGroup || e.b.pend.get(salesAdmin) != nil {
		t.Fatalf("s1 group = %q", group("s1"))
	}
	for _, m := range e.since(n) {
		if m.Method == "editMessageText" {
			t.Fatalf("a group editor was shown to a limited admin: %q", m.Text)
		}
	}

	// The full administrator can still make any group, and gets the group step.
	e.press(fullAdmin, "c:new")
	e.say(fullAdmin, "full1")
	if p = e.b.pend.get(fullAdmin); p == nil || p.key != "grp" {
		t.Fatalf("full admin wizard: %+v", p)
	}
	e.press(fullAdmin, "w:grp:#new")
	e.say(fullAdmin, "Brand new")
	e.press(fullAdmin, "w:vol:1")
	e.press(fullAdmin, "w:days:0")
	e.press(fullAdmin, "w:ip:0")
	if group("full1") != "Brand new" {
		t.Fatalf("full1 group = %q", group("full1"))
	}
}

func TestScopedBulkEditAndCleanupStayInTheGroup(t *testing.T) {
	e := newScopeEnv(t)
	volume := func(name string) int64 { return e.client(name).Volume }
	o1Before := volume("o1")

	// Bulk "add volume" to "All" changes the group's limited clients only.
	e.press(salesAdmin, "c:bk:fa:vol")
	if p := e.b.pend.get(salesAdmin); p == nil || p.kind != "bk.vol" {
		t.Fatalf("bulk volume prompt: %+v", p)
	}
	e.say(salesAdmin, "5")
	if volume("s1") != 6*gib || volume("o1") != o1Before || volume("od") != depletedVol {
		t.Fatalf("after bulk volume: s1=%d o1=%d od=%d", volume("s1"), volume("o1"), volume("od"))
	}
	// Disable / enable them all.
	e.press(salesAdmin, "c:bk:fa:dis:y")
	for name, want := range map[string]bool{"s1": false, "s2": false, "o1": true, "n1": true, "cl1": true} {
		if e.client(name).Enable != want {
			t.Errorf("%s enable = %v after bulk disable, want %v", name, !want, want)
		}
	}
	e.press(salesAdmin, "c:bk:fa:en:y")
	if !e.client("s1").Enable {
		t.Error("bulk enable did not enable s1")
	}

	// The group scopes of the bulk dialog are the group's spellings ("Sales"
	// and "sales" are two entries there, as in the full list), and an index
	// beyond them reaches nobody.
	if got := clientNames(e.b.as(salesAdmin).scopeClients("g0")); got != "s1,sd" {
		t.Fatalf("group scope 0 = %s", got)
	}
	if got := clientNames(e.b.as(salesAdmin).scopeClients("g1")); got != "s2" {
		t.Fatalf("group scope 1 = %s", got)
	}
	if got := clientNames(e.b.as(salesAdmin).scopeClients("g2")); got != "" {
		t.Fatalf("a third group scope reached %s", got)
	}
	if got := clientNames(e.b.as(salesAdmin).scopeClients("fa")); got != "s1,s2,sd" {
		t.Fatalf("all scope = %s", got)
	}

	// Cleanup lists and deletes the group's depleted clients only. (The bulk
	// volume above topped sd up, so make two expired ones.)
	for _, row := range []struct{ name, group string }{{"sx", salesGroup}, {"ox", otherGroup}} {
		c := seedClient(t, row.name, row.group, 0, 0)
		c.Expiry = 1
		if err := database.GetDB().Save(&c).Error; err != nil {
			t.Fatal(err)
		}
	}
	n := len(e.got())
	e.press(salesAdmin, "c:clean")
	if text := lastText(e.since(n), "editMessageText"); !strings.Contains(text, "sx") || strings.Contains(text, "ox") || strings.Contains(text, "od") {
		t.Fatalf("cleanup preview:\n%s", text)
	}
	e.press(salesAdmin, "c:cleany")
	if e.exists("sx") || !e.exists("ox") || !e.exists("od") || !e.exists("s1") || !e.exists("sd") {
		t.Fatalf("cleanup: sx=%v ox=%v od=%v s1=%v sd=%v", e.exists("sx"), e.exists("ox"), e.exists("od"), e.exists("s1"), e.exists("sd"))
	}

	// Bulk delete of "All" deletes the group and nothing else.
	e.press(salesAdmin, "c:bk:fa:del:y")
	if e.exists("s1") || e.exists("s2") {
		t.Fatal("bulk delete left the group's clients")
	}
	for _, name := range []string{"o1", "od", "n1", "cl1"} {
		if !e.exists(name) {
			t.Fatalf("bulk delete by a limited admin removed %s", name)
		}
	}
}

func TestAlertsAreRoutedByGroup(t *testing.T) {
	e := newScopeEnv(t)
	ctx := context.Background()
	// Push the clients over the alert threshold: 95% of their volume.
	for _, name := range []string{"s1", "o1"} {
		c := e.client(name)
		c.Volume, c.Up = 100, 95
		if err := database.GetDB().Save(&c).Error; err != nil {
			t.Fatal(err)
		}
	}
	e.b.checkClients(ctx, map[string]bool{})
	byChat := map[int64]string{}
	for _, m := range e.got() {
		byChat[m.ChatID] += m.Text + "\n"
	}
	if text := byChat[fullAdmin]; !strings.Contains(text, "s1") || !strings.Contains(text, "o1") || !strings.Contains(text, "sd") || !strings.Contains(text, "od") {
		t.Fatalf("full admin alert:\n%s", text)
	}
	if text := byChat[salesAdmin]; !strings.Contains(text, "s1") || !strings.Contains(text, "sd") || strings.Contains(text, "o1") || strings.Contains(text, "od") {
		t.Fatalf("limited admin alert:\n%s", text)
	}

	// Panel-wide notices and backups go to full administrators only.
	n := len(e.got())
	e.b.broadcast(ctx, "core down", "home")
	e.b.announce(ctx)
	got := map[int64][]string{}
	for _, m := range e.since(n) {
		got[m.ChatID] = append(got[m.ChatID], m.Text)
	}
	if len(got[fullAdmin]) != 2 || len(got[salesAdmin]) != 1 || got[salesAdmin][0] != e.b.t("started") {
		t.Fatalf("broadcast / announce routing: %v", got)
	}
	if fmt.Sprint(e.b.access().with("home")) != "[42]" {
		t.Fatalf("admins with the home section = %v", e.b.access().with("home"))
	}
	n = len(e.got())
	e.b.as(salesAdmin).sendBackup(ctx, salesAdmin)
	if msgs := e.since(n); len(msgs) != 1 || msgs[0].Method != "sendMessage" {
		t.Fatalf("a limited admin got a backup: %+v", msgs)
	}
	e.say(salesAdmin, "/backup")
	e.press(salesAdmin, "m:backup")
	for _, m := range e.got() {
		if m.Method == "sendDocument" {
			t.Fatal("a database backup was sent to a limited admin")
		}
	}
}

func TestScopedAdminHelpAndCommandList(t *testing.T) {
	e := newScopeEnv(t)
	n := len(e.got())
	e.say(salesAdmin, "/help")
	help := lastText(e.since(n), "sendMessage")
	if !strings.Contains(help, "Sales") || strings.Contains(help, "/backup") || strings.Contains(help, "/restart") || strings.Contains(help, "/inbounds") {
		t.Fatalf("limited help:\n%s", help)
	}
	n = len(e.got())
	e.say(fullAdmin, "/help")
	if help = lastText(e.since(n), "sendMessage"); !strings.Contains(help, "/backup") {
		t.Fatalf("full help:\n%s", help)
	}
	// Every command the limited administrator is told about is one they may use.
	for _, c := range adminCommands {
		if scopedCommands[c.name] {
			continue
		}
		switch c.name {
		case "stats", "settings", "changes", "nodes", "inbounds", "traffic", "backup", "logs", "sync", "restart", "maintenance", "shop", "sales":
		default:
			t.Errorf("command /%s is hidden from limited admins; is that intended?", c.name)
		}
	}
	n = len(e.got())
	e.say(salesAdmin, "/start")
	msgs := e.since(n)
	if len(msgs) != 1 || !hasData(msgs[0].Markup.Keyboard, "c:ls:a:0") || hasData(msgs[0].Markup.Keyboard, "o:in:ls:0") {
		t.Fatalf("/start for a limited admin: %+v", msgs)
	}
}
