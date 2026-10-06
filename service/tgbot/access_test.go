package tgbot

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

const (
	ownerID     = int64(1000) // the bot owner
	clientsOnly = int64(55)   // custom: clients and stats
	noSections  = int64(56)   // custom: nothing yet
	nodesOnly   = int64(57)   // custom: nodes
	coreOnly    = int64(58)   // custom: core
)

// configure stores settings the way the web panel does and lets the bot take
// the new rights over, as the supervisor does within seconds.
func (e *scopeEnv) configure(values map[string]string) {
	e.t.Helper()
	raw, _ := json.Marshal(values)
	if err := (&service.SettingService{}).Save(database.GetDB(), raw); err != nil {
		e.t.Fatal(err)
	}
	if err := e.b.reloadAccess(); err != nil {
		e.t.Fatal(err)
	}
}

// newAccessEnv is the scope environment with an owner (1000), a full
// administrator (42), a group-limited one (77, Sales) and custom ones.
func newAccessEnv(t *testing.T) *scopeEnv {
	t.Helper()
	e := newScopeEnv(t)
	e.configure(map[string]string{
		"tgBotOwner":  strconv.FormatInt(ownerID, 10),
		"tgBotAdmins": "42",
		"tgBotScopes": "77=Sales",
		"tgBotPerms":  "55=clients,stats\n56=\n57=nodes\n58=core",
	})
	return e
}

// accessSettings are the three settings the Admins screen edits.
func accessSettings(t *testing.T) (admins, scopes, perms string) {
	t.Helper()
	s, err := (&service.SettingService{}).GetTgBotSettings()
	if err != nil {
		t.Fatal(err)
	}
	return s.Admins, s.Scopes, s.Perms
}

func keysOf(set sectionSet) string { return strings.Join(set.keys(), ",") }

func TestParsePerms(t *testing.T) {
	perms, bad := parsePerms("55=clients, stats\n56 = Logs;BACKUP\n57=\n58=clients,nonsense,,home\nnoequals\n59 x=clients\n55=logs\n# comment\n=nothing\n0=clients\nabc=clients\n-5=home\n60\n")
	want := map[int64]string{55: "clients,stats", 56: "logs,backup", 57: "", 58: "home,clients", -5: "home"}
	if len(perms) != len(want) {
		t.Fatalf("perms = %v", perms)
	}
	for id, keys := range want {
		set, ok := perms[id]
		if !ok || keysOf(set) != keys {
			t.Errorf("perms[%d] = %v (%v), want %q", id, set, ok, keys)
		}
	}
	if fmt.Sprint(bad) != "[59 60]" {
		t.Fatalf("bad = %v, want [59 60]", bad)
	}
	// A good line wins over a broken one for the same ID.
	if perms, bad := parsePerms("61\n61=logs"); len(bad) != 0 || keysOf(perms[61]) != "logs" {
		t.Fatalf("perms = %v, bad = %v", perms, bad)
	}
	if perms, bad := parsePerms(""); len(perms) != 0 || len(bad) != 0 {
		t.Fatal("empty setting gave perms")
	}
}

func TestBuildAccess(t *testing.T) {
	perms := map[int64]sectionSet{55: {"clients": true}, ownerID: {"logs": true}, 56: {}}
	scopes := map[int64]string{77: "Sales", ownerID: "Sales", 55: "Team"}
	acc := buildAccess(ownerID, []int64{42, ownerID}, scopes, perms, []int64{88, ownerID})
	if fmt.Sprint(acc.order) != "[1000 42 55 77 56 88]" {
		t.Fatalf("order = %v", acc.order)
	}
	if !acc.roleOf(ownerID).full() || !acc.isOwner(ownerID) || acc.isOwner(42) {
		t.Fatal("the owner is limited by a line about them, or somebody else is the owner")
	}
	if !acc.roleOf(42).full() {
		t.Fatalf("42 = %+v", acc.roleOf(42))
	}
	if r := acc.roleOf(55); r.group != "Team" || r.sections != nil {
		t.Fatalf("a group limit must beat a section list, got %+v", r)
	}
	if r := acc.roleOf(77); r.group != "Sales" {
		t.Fatalf("77 = %+v", r)
	}
	for _, id := range []int64{56, 88} {
		if r := acc.roleOf(id); r.group != "" || r.sections == nil || len(r.sections) != 0 {
			t.Fatalf("%d should have no access, got %+v", id, r)
		}
	}
	if !acc.isMember(88) || acc.isMember(999) {
		t.Fatal("membership is wrong")
	}
	if got := acc.with("clients"); fmt.Sprint(got) != "[1000 42]" {
		t.Fatalf("with(clients) = %v", got)
	}
	if got := acc.with("logs", "clients"); fmt.Sprint(got) != "[1000 42]" {
		t.Fatalf("with(logs, clients) = %v", got)
	}

	// Without an owner nobody is the owner, whatever the lines say.
	acc = buildAccess(0, []int64{42}, nil, nil, nil)
	if acc.isOwner(0) || acc.isOwner(42) || fmt.Sprint(acc.order) != "[42]" {
		t.Fatalf("no owner: %+v", acc)
	}
}

func TestOwnerSetting(t *testing.T) {
	for in, want := range map[string]int64{"1000": 1000, " 1000 ": 1000, "": 0, "abc": 0, "-5": 0, "0": 0, "12 34": 0} {
		if got := parseOwner(in); got != want {
			t.Errorf("parseOwner(%q) = %d, want %d", in, got, want)
		}
	}
	testBot(t)
	save := func(values map[string]string) error {
		raw, _ := json.Marshal(values)
		return (&service.SettingService{}).Save(database.GetDB(), raw)
	}
	for _, bad := range []string{"abc", "-5", "0", "12 34", "1.5"} {
		if err := save(map[string]string{"tgBotOwner": bad}); err == nil {
			t.Errorf("the owner %q was accepted", bad)
		}
	}
	if err := save(map[string]string{"tgBotOwner": "1000"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig()
	if err != nil || cfg.Owner != 1000 || !cfg.access().isOwner(1000) {
		t.Fatalf("owner = %d (%v)", cfg.Owner, err)
	}
	if err := save(map[string]string{"tgBotOwner": ""}); err != nil {
		t.Fatal(err)
	}
	if cfg, _ = loadConfig(); cfg.Owner != 0 {
		t.Fatal("clearing the owner kept it")
	}
}

func TestEverySectionIsWiredUp(t *testing.T) {
	for _, k := range kindList {
		if kindSection[k.code] == "" {
			t.Errorf("object kind %q has no section", k.code)
		}
	}
	for code, sec := range kindSection {
		if !sectionKnown(sec) {
			t.Errorf("kind %q points at the unknown section %q", code, sec)
		}
	}
	for cmd, sec := range commandSections {
		if !sectionKnown(sec) {
			t.Errorf("/%s points at the unknown section %q", cmd, sec)
		}
	}
	open := map[string]bool{"menu": true, "help": true}
	for _, c := range adminCommands {
		if _, gated := commandSections[c.name]; !gated && !open[c.name] {
			t.Errorf("/%s belongs to no section, so every administrator may use it; add it to commandSections or to this list", c.name)
		}
	}

	// Every button of the main menu belongs to a section, and every section is
	// on the main menu.
	b, _ := fakeTelegram(t)
	reachable := map[string]bool{}
	for _, row := range b.mainMenu() {
		for _, btn := range row {
			sec := callbackSection(strings.Split(btn.Data, ":"))
			if sec == secDeny {
				t.Errorf("the menu button %q belongs to no section", btn.Data)
			}
			reachable[sec] = true
		}
	}
	for _, d := range sectionDefs {
		if !reachable[d.key] {
			t.Errorf("section %q has no button on the main menu", d.key)
		}
	}
	for data, want := range map[string]string{
		"h:home": "home", "c:ls:a:0": "clients", "c:view:3": "clients", "w:vol:10": "clients", "o:in:ls:0": "inbounds", "o:out:v:1": "outbounds", "o:ep:ls:0": "endpoints",
		"o:sv:ls:0": "services", "o:tl:ls:0": "tls", "o:nd:ls:0": "nodes", "g:rl:ls:0": "routing", "g:dn:ls:0": "routing", "g:basics:ls:0": "routing",
		"s:ls": "settings", "m:prest": "settings", "m:prestY": "settings", "t:d1": "stats", "x:ls": "stats", "m:traffic": "stats", "m:logs:info": "logs",
		"m:backup": "backup", "m:sync": "nodes", "m:maint": "core", "m:restart": "core", "m:restarty": "core", "m:status": "home", "m:menu": secNone, "x:cancel": secNone,
		"u:sub:5": secNone, "u:view:5": secNone, "a:ls": "admins", "a:add": secOwner, "a:e:5": secOwner, "a:rmy:5": secOwner, "a:q:5": secOwner, "a:qa:5:10": secOwner, "a:qt:5": secOwner, "a:qry:5": secOwner, "bogus:x": secDeny, "m:bogus": secDeny, "o:zz:ls:0": secDeny,
	} {
		if got := callbackSection(strings.Split(data, ":")); got != want {
			t.Errorf("callbackSection(%q) = %q, want %q", data, got, want)
		}
	}
}

func TestCustomAdminReachesOnlyTheirSections(t *testing.T) {
	e := newAccessEnv(t)
	denied := e.b.t("noAccess")

	// Commands: the clients and stats ones work; the rest are refused.
	for _, cmd := range []string{"clients", "online", "stats", "traffic", "changes", "client s1", "ips s1"} {
		n := len(e.got())
		e.say(clientsOnly, "/"+cmd)
		if text := lastText(e.since(n), "sendMessage"); text == denied || text == "" {
			t.Errorf("/%s was refused: %q", cmd, text)
		}
	}
	for _, cmd := range []string{"inbounds", "nodes", "settings", "backup", "logs", "sync", "restart", "maintenance on", "home", "status"} {
		n := len(e.got())
		e.say(clientsOnly, "/"+cmd)
		msgs := e.since(n)
		if len(msgs) != 1 || msgs[0].Text != denied {
			t.Errorf("/%s answered %+v", cmd, msgs)
		}
	}
	if on, _ := (&service.SettingService{}).GetMaintenance(); !on {
		t.Fatal("maintenance mode was switched off")
	}
	for _, m := range e.got() {
		if m.Method == "sendDocument" {
			t.Fatal("a backup was sent to an admin without the Backup section")
		}
	}

	// Buttons.
	for _, data := range []string{"c:ls:a:0", "t:d1", "x:ls", "m:clients", "m:online", "m:traffic", "m:menu"} {
		n := len(e.got())
		e.press(clientsOnly, data)
		for _, m := range e.since(n) {
			if m.Method == "answerCallbackQuery" && m.Text == denied {
				t.Errorf("%s was refused", data)
			}
		}
	}
	for _, data := range []string{"h:home", "o:in:ls:0", "o:nd:ls:0", "o:out:ls:0", "g:rl:ls:0", "g:basics:ls:0", "s:ls", "s:ask:webPort", "m:backup", "m:restarty", "m:maint", "m:logs:info", "m:sync", "m:status", "m:prestY", "a:ls", "a:e:77", "a:add", "bogus:x"} {
		n := len(e.got())
		e.press(clientsOnly, data)
		msgs := e.since(n)
		if len(msgs) != 1 || msgs[0].Method != "answerCallbackQuery" || msgs[0].Text != denied {
			t.Errorf("%s answered %+v", data, msgs)
		}
	}

	// The menu offers exactly their buttons.
	n := len(e.got())
	e.say(clientsOnly, "/menu")
	kb := keyboardOf(e.since(n))
	for _, want := range []string{"c:ls:a:0", "t:d1", "x:ls"} {
		if !hasData(kb, want) {
			t.Errorf("the menu lacks %s: %v", want, callbackData(kb))
		}
	}
	for _, not := range []string{"h:home", "o:in:ls:0", "g:rl:ls:0", "s:ls", "m:backup", "m:restart", "a:ls", "m:logs:info"} {
		if hasData(kb, not) {
			t.Errorf("the menu offers %s", not)
		}
	}

	// Typed answers need the section too.
	e.b.pend.set(clientsOnly, &pending{kind: "set", key: "webPort", data: map[string]string{}})
	e.say(clientsOnly, "9999")
	if port, _ := (&service.SettingService{}).GetPort(); port == 9999 {
		t.Fatal("an admin without Settings changed a panel setting")
	}
	if e.b.pend.get(clientsOnly) != nil {
		t.Fatal("the foreign prompt stayed armed")
	}
	e.b.pend.set(clientsOnly, &pending{kind: "obj.new", key: "in", data: map[string]string{}})
	e.say(clientsOnly, `{"type":"mixed","tag":"evil","listen":"::","listen_port":9999}`)
	var inbounds int64
	database.GetDB().Model(model.Inbound{}).Count(&inbounds)
	if inbounds != 0 {
		t.Fatal("an admin without Inbounds created an inbound")
	}

	// They can use what they have, all of it: nothing on the client screens is
	// quietly hidden from them.
	e.say(clientsOnly, "/add zed 1 1")
	if !e.exists("zed") {
		t.Fatal("an admin with Clients could not add a client")
	}
	as := e.b.as(clientsOnly)
	c := e.client("s1")
	_, card := as.card(c)
	if got, want := fmt.Sprint(as.filterKeyboard(card)), fmt.Sprint(card); got != want {
		t.Fatalf("the client card lost buttons for an admin with Clients:\n%s\n%s", got, want)
	}
	list, _ := as.clientsScreen("a", 0)
	_ = list
	if _, kb := as.clientsScreen("a", 0); fmt.Sprint(as.filterKeyboard(kb)) != fmt.Sprint(kb) {
		t.Fatal("the client list lost buttons for an admin with Clients")
	}
}

func TestAdminWithoutSectionsGetsAnExplanation(t *testing.T) {
	e := newAccessEnv(t)
	for _, text := range []string{"/start", "/menu", "/help"} {
		n := len(e.got())
		e.say(noSections, text)
		msgs := e.since(n)
		if len(msgs) != 1 || msgs[0].Text == "" || !strings.Contains(msgs[0].Text, "any section") || msgs[0].Markup != nil {
			t.Errorf("%s answered %+v", text, msgs)
		}
	}
	n := len(e.got())
	e.say(noSections, "/clients")
	if msgs := e.since(n); len(msgs) != 1 || msgs[0].Text != e.b.t("noAccess") {
		t.Fatalf("/clients answered %+v", msgs)
	}
	e.press(noSections, "m:menu")
	if txt := lastText(e.got(), "editMessageText"); !strings.Contains(txt, "any section") {
		t.Fatalf("the menu button showed %q", txt)
	}
	// Anyone can look up their ID.
	n = len(e.got())
	e.say(noSections, "/id")
	if msgs := e.since(n); len(msgs) != 1 || !strings.Contains(msgs[0].Text, "56") {
		t.Fatalf("/id answered %+v", msgs)
	}
}

func TestCustomAdminHelpListsTheirCommands(t *testing.T) {
	e := newAccessEnv(t)
	n := len(e.got())
	e.say(clientsOnly, "/help")
	help := lastText(e.since(n), "sendMessage")
	for _, want := range []string{"/clients", "/stats", "/add", "/menu", "/id"} {
		if !strings.Contains(help, want) {
			t.Errorf("help lacks %s:\n%s", want, help)
		}
	}
	for _, not := range []string{"/backup", "/restart", "/inbounds", "/settings", "/logs", "/nodes"} {
		if strings.Contains(help, not) {
			t.Errorf("help offers %s:\n%s", not, help)
		}
	}
	// The same list goes to Telegram's command menu.
	var names []string
	for _, c := range e.b.commandsFor(clientsOnly) {
		names = append(names, c.name)
	}
	list := strings.Join(names, " ")
	if !strings.Contains(list, "clients") || !strings.Contains(list, "stats") || strings.Contains(list, "backup") || strings.Contains(list, "restart") || !strings.Contains(list, "id") {
		t.Fatalf("command menu = %s", list)
	}
	// Full administrators get everything; group-limited ones their own list.
	if got := len(e.b.commandsFor(fullAdmin)); got != len(adminCommands)+1 {
		t.Fatalf("full admin has %d commands", got)
	}
	for _, c := range e.b.commandsFor(salesAdmin) {
		if c.name != "id" && !scopedCommands[c.name] {
			t.Errorf("limited admin is shown /%s", c.name)
		}
	}
}

func TestOwnerHasEverythingAndIsNotLimitedByLines(t *testing.T) {
	e := newAccessEnv(t)
	// Lines about the owner are ignored, and the owner needs no admin entry.
	e.configure(map[string]string{"tgBotScopes": "77=Sales\n1000=Sales", "tgBotPerms": "1000=logs"})
	as := e.b.as(ownerID)
	if as.scope != "" || as.sections != nil || !as.owner {
		t.Fatalf("owner = %+v", as)
	}
	n := len(e.got())
	e.say(ownerID, "/inbounds")
	if text := lastText(e.since(n), "sendMessage"); text == e.b.t("noAccess") || text == "" {
		t.Fatalf("/inbounds for the owner: %q", text)
	}
	n = len(e.got())
	e.say(ownerID, "/menu")
	kb := keyboardOf(e.since(n))
	for _, want := range []string{"o:in:ls:0", "m:backup", "a:ls", "s:ls", "m:restart"} {
		if !hasData(kb, want) {
			t.Errorf("the owner's menu lacks %s", want)
		}
	}
}

func TestOnlyTheOwnerCanEditAdmins(t *testing.T) {
	e := newAccessEnv(t)
	adminsBefore, scopesBefore, permsBefore := accessSettings(t)
	buttons := []string{"a:e:55", "a:t:55:logs", "a:p:55:full", "a:g:55", "a:sg:55:0", "a:sg:55:#new", "a:rm:55", "a:rmy:55", "a:add", "a:e:77", "a:p:77:none", "a:rmy:42", "a:t:42:logs",
		"a:q:55", "a:qa:55:50", "a:qt:55", "a:qr:55", "a:qry:55", "a:qc:55", "a:qcy:55"}
	for _, who := range []int64{fullAdmin, salesAdmin, clientsOnly, noSections, 999} {
		for _, data := range buttons {
			n := len(e.got())
			e.press(who, data)
			msgs := e.since(n)
			if len(msgs) != 1 || msgs[0].Method != "answerCallbackQuery" || msgs[0].Text == "" {
				t.Errorf("%d pressing %s got %+v", who, data, msgs)
			}
		}
		// A prompt only the owner could have opened does nothing for them.
		e.b.pend.set(who, &pending{kind: "ad.add", data: map[string]string{}})
		e.say(who, "900")
		e.b.pend.set(who, &pending{kind: "ad.grp", data: map[string]string{"id": "55"}})
		e.say(who, "Sneaky")
		e.b.pend.set(who, &pending{kind: "ad.quota", data: map[string]string{"id": "55"}})
		e.say(who, "99999")
		e.b.pend.clear(who)
	}
	if a, s, p := accessSettings(t); a != adminsBefore || s != scopesBefore || p != permsBefore {
		t.Fatalf("somebody but the owner changed the admins:\n%q %q %q\n%q %q %q", a, s, p, adminsBefore, scopesBefore, permsBefore)
	}
	if e.b.access().isMember(900) {
		t.Fatal("a non-owner added an admin")
	}

	// A full administrator still sees the list, read-only.
	n := len(e.got())
	e.press(fullAdmin, "a:ls")
	msgs := e.since(n)
	text, kb := lastText(msgs, "editMessageText"), keyboardOf(msgs)
	if !strings.Contains(text, "55") || !strings.Contains(text, "Only the bot owner") {
		t.Fatalf("the admins screen for a full admin:\n%s", text)
	}
	for _, data := range callbackData(kb) {
		if strings.HasPrefix(data, "a:") {
			t.Errorf("a non-owner is offered %s", data)
		}
	}
}

func TestWithoutAnOwnerNobodyEditsAdmins(t *testing.T) {
	e := newScopeEnv(t)
	e.configure(map[string]string{"tgBotAdmins": "42, 77", "tgBotScopes": "77=Sales"})
	for _, who := range []int64{fullAdmin, 0, 1000} {
		n := len(e.got())
		e.press(who, "a:add")
		if msgs := e.since(n); len(msgs) != 1 || msgs[0].Method != "answerCallbackQuery" || msgs[0].Text == "" {
			t.Errorf("%d got %+v", who, msgs)
		}
	}
	n := len(e.got())
	e.press(fullAdmin, "a:ls")
	msgs := e.since(n)
	if text := lastText(msgs, "editMessageText"); !strings.Contains(text, "Bot owner") {
		t.Fatalf("no hint about setting the owner:\n%s", text)
	}
	for _, data := range callbackData(keyboardOf(msgs)) {
		if strings.HasPrefix(data, "a:") {
			t.Errorf("offered %s without an owner", data)
		}
	}
	// Asking the access layer directly does not make anybody the owner.
	if e.b.as(0).owner || e.b.as(fullAdmin).owner {
		t.Fatal("somebody is the owner of an ownerless bot")
	}
}

func adminButtons(kb [][]button) []string {
	var out []string
	for _, d := range callbackData(kb) {
		if strings.HasPrefix(d, "a:") {
			out = append(out, d)
		}
	}
	return out
}

func TestOwnerManagesAdminsFromTheBot(t *testing.T) {
	e := newAccessEnv(t)
	ctx := context.Background()
	fakeChat(clientsOnly, "Ali Rezai")

	// The list: everybody, with names where Telegram knows them, and edit
	// buttons for everybody but the owner.
	n := len(e.got())
	e.press(ownerID, "a:ls")
	msgs := e.since(n)
	text, kb := lastText(msgs, "editMessageText"), keyboardOf(msgs)
	for _, want := range []string{"👑", "1000", "owner", "full access", "group: <b>Sales</b>", "Ali Rezai", "⛔", "no access"} {
		if !strings.Contains(text, want) {
			t.Errorf("the admins screen lacks %q:\n%s", want, text)
		}
	}
	for _, want := range []string{"a:e:42", "a:e:77", "a:e:55", "a:e:56", "a:add"} {
		if !hasData(kb, want) {
			t.Errorf("the admins screen lacks %s: %v", want, callbackData(kb))
		}
	}
	if hasData(kb, "a:e:1000") {
		t.Error("the owner can be edited")
	}
	if !strings.Contains(fmt.Sprint(kb), "Ali Rezai") {
		t.Errorf("the edit button does not carry the name: %v", kb)
	}

	// Add an administrator: bad answers keep the prompt open.
	e.press(ownerID, "a:add")
	if p := e.b.pend.get(ownerID); p == nil || p.kind != "ad.add" {
		t.Fatalf("pending = %+v", p)
	}
	for _, bad := range []string{"abc", "-4", "0", "42", "1000", "77"} {
		n = len(e.got())
		e.say(ownerID, bad)
		if text := lastText(e.since(n), "editMessageText"); !strings.Contains(text, "Failed") {
			t.Errorf("%q was accepted: %q", bad, text)
		}
		if e.b.pend.get(ownerID) == nil {
			t.Fatalf("the prompt closed after %q", bad)
		}
	}
	if e.b.access().isMember(900) {
		t.Fatal("900 is a member already")
	}
	n = len(e.got())
	e.say(ownerID, " 900 ")
	msgs = e.since(n)
	text, kb = lastText(msgs, "editMessageText"), keyboardOf(msgs)
	if !e.b.access().isMember(900) || e.b.pend.get(ownerID) != nil {
		t.Fatalf("900 was not added: %v", e.b.access().order)
	}
	if !strings.Contains(text, "900") || !strings.Contains(text, "no access") || !hasData(kb, "a:t:900:clients") || !hasData(kb, "a:rm:900") || !hasData(kb, "a:p:900:full") {
		t.Fatalf("the edit screen of the new admin:\n%s\n%v", text, callbackData(kb))
	}
	// The edit screen offers every section, all off.
	toggles := 0
	for _, d := range sectionDefs {
		if !hasData(kb, "a:t:900:"+d.key) {
			t.Errorf("no toggle for %s", d.key)
		}
		toggles++
	}
	if strings.Count(fmt.Sprint(kb), "⬜") != toggles {
		t.Errorf("a new admin should have every section off: %v", kb)
	}
	admins, _, perms := accessSettings(t)
	if !strings.Contains(admins, "900") || !strings.Contains(perms, "900=\n") && !strings.HasSuffix(perms, "900=") {
		t.Fatalf("settings after adding: admins %q perms %q", admins, perms)
	}
	// The change was recorded like any other settings edit, under the owner's ID,
	// and applied live.
	var changes []model.Changes
	database.GetDB().Where("key = ? AND actor = ?", "settings", "telegram:"+strconv.FormatInt(ownerID, 10)).Find(&changes)
	if len(changes) == 0 {
		t.Fatal("the edit is not in the change history")
	}
	n = len(e.got())
	e.say(900, "/clients")
	if msgs := e.since(n); len(msgs) != 1 || msgs[0].Text != e.b.t("noAccess") {
		t.Fatalf("a new admin has access: %+v", msgs)
	}

	// Switch sections on, one at a time; they work at once.
	e.press(ownerID, "a:t:900:clients")
	if text := lastEditText(e); !strings.Contains(text, "900") {
		t.Fatalf("edit screen after a toggle:\n%s", text)
	}
	if _, _, perms = accessSettings(t); !strings.Contains(perms, "900=clients") {
		t.Fatalf("perms = %q", perms)
	}
	n = len(e.got())
	e.say(900, "/clients")
	if text := lastText(e.since(n), "sendMessage"); text == e.b.t("noAccess") || text == "" {
		t.Fatalf("/clients is still refused: %q", text)
	}
	e.press(ownerID, "a:t:900:logs")
	e.press(ownerID, "a:t:900:backup")
	if _, _, perms = accessSettings(t); !strings.Contains(perms, "900=clients,logs,backup") {
		t.Fatalf("perms = %q", perms)
	}
	// Off again.
	e.press(ownerID, "a:t:900:logs")
	if _, _, perms = accessSettings(t); !strings.Contains(perms, "900=clients,backup") {
		t.Fatalf("perms = %q", perms)
	}
	n = len(e.got())
	e.say(900, "/logs")
	if msgs := e.since(n); len(msgs) != 1 || msgs[0].Text != e.b.t("noAccess") {
		t.Fatalf("/logs after switching Logs off: %+v", msgs)
	}

	// Presets.
	e.press(ownerID, "a:p:900:full")
	if a, s, p := accessSettings(t); !strings.Contains(a, "900") || strings.Contains(p, "900") || strings.Contains(s, "900") {
		t.Fatalf("full: %q %q %q", a, s, p)
	}
	if r := e.b.access().roleOf(900); !r.full() {
		t.Fatalf("role = %+v", r)
	}
	n = len(e.got())
	e.say(900, "/inbounds")
	if text := lastText(e.since(n), "sendMessage"); text == e.b.t("noAccess") || text == "" {
		t.Fatalf("a full admin cannot use /inbounds: %q", text)
	}
	e.press(ownerID, "a:p:900:cl")
	if _, _, perms = accessSettings(t); !strings.Contains(perms, "900=clients") || strings.Contains(perms, "900=clients,") {
		t.Fatalf("perms = %q", perms)
	}
	e.press(ownerID, "a:p:900:none")
	if _, _, perms = accessSettings(t); !strings.Contains(perms, "900=") || strings.Contains(perms, "900=c") {
		t.Fatalf("perms = %q", perms)
	}
	if r := e.b.access().roleOf(900); r.sections == nil || len(r.sections) != 0 {
		t.Fatalf("role = %+v", r)
	}

	// A full administrator is edited by switching a section off.
	e.press(ownerID, "a:t:42:logs")
	r := e.b.access().roleOf(fullAdmin)
	if r.sections == nil || r.sections["logs"] || len(r.sections) != len(sectionDefs)-1 {
		t.Fatalf("42 after switching Logs off: %+v", r)
	}
	n = len(e.got())
	e.say(fullAdmin, "/logs")
	if msgs := e.since(n); len(msgs) != 1 || msgs[0].Text != e.b.t("noAccess") {
		t.Fatalf("/logs for 42: %+v", msgs)
	}
	n = len(e.got())
	e.say(fullAdmin, "/inbounds")
	if text := lastText(e.since(n), "sendMessage"); text == e.b.t("noAccess") {
		t.Fatal("42 lost more than Logs")
	}
	e.press(ownerID, "a:p:42:full")
	if !e.b.access().roleOf(fullAdmin).full() {
		t.Fatal("42 is not full again")
	}
	_ = ctx
}

func lastEditText(e *scopeEnv) string {
	e.t.Helper()
	edits := texts(e.got(), "editMessageText")
	if len(edits) == 0 {
		e.t.Fatal("no message was edited")
	}
	return edits[len(edits)-1]
}

func TestOwnerLimitsAnAdminToAGroup(t *testing.T) {
	e := newAccessEnv(t)
	n := len(e.got())
	e.press(ownerID, "a:g:55")
	msgs := e.since(n)
	kb := keyboardOf(msgs)
	var otherBtn string
	for _, row := range kb {
		for _, btn := range row {
			if strings.Contains(btn.Text, "Other") {
				otherBtn = btn.Data
			}
			if strings.HasSuffix(btn.Data, ":-") {
				t.Errorf("the chooser offers “no group”: %v", btn)
			}
		}
	}
	if otherBtn == "" || !hasData(kb, "a:sg:55:#new") || !hasData(kb, "a:e:55") {
		t.Fatalf("group chooser: %v", callbackData(kb))
	}
	if p := e.b.pend.get(ownerID); p == nil || p.kind != "ad.grp" {
		t.Fatalf("pending = %+v", p)
	}

	e.press(ownerID, otherBtn)
	if _, scopes, perms := accessSettings(t); !strings.Contains(scopes, "55=Other") || strings.Contains(perms, "55=") {
		t.Fatalf("scopes %q perms %q", scopes, perms)
	}
	if as := e.b.as(clientsOnly); as.scope != "Other" || as.sections != nil {
		t.Fatalf("55 = %+v", as)
	}
	// Their view is that group now, and only clients.
	n = len(e.got())
	e.say(clientsOnly, "/clients o1")
	if text := lastText(e.since(n), "sendMessage"); !strings.Contains(text, "o1") {
		t.Fatalf("55 cannot see their group: %q", text)
	}
	n = len(e.got())
	e.say(clientsOnly, "/clients s1")
	if text := lastText(e.since(n), "sendMessage"); strings.Contains(text, "s1") && !strings.Contains(text, "No") {
		t.Fatalf("55 sees another group: %q", text)
	}
	n = len(e.got())
	e.say(clientsOnly, "/stats")
	if msgs := e.since(n); len(msgs) != 1 || msgs[0].Text != e.b.t("scopeDenied") {
		t.Fatalf("/stats for a group-limited admin: %+v", msgs)
	}
	// The edit screen of a limited admin has presets but no section toggles.
	n = len(e.got())
	e.press(ownerID, "a:e:55")
	kb = keyboardOf(e.since(n))
	if hasData(kb, "a:t:55:clients") || !hasData(kb, "a:p:55:full") || !hasData(kb, "a:g:55") {
		t.Fatalf("edit screen of a limited admin: %v", callbackData(kb))
	}
	if text := lastEditText(e); !strings.Contains(text, "group: <b>Other</b>") {
		t.Fatalf("edit screen:\n%s", text)
	}
	// Toggling a section of a limited admin is refused rather than lifting the limit.
	n = len(e.got())
	e.press(ownerID, "a:t:55:logs")
	if msgs := e.since(n); len(msgs) != 1 || msgs[0].Method != "answerCallbackQuery" || msgs[0].Text == "" {
		t.Fatalf("toggle on a limited admin: %+v", msgs)
	}
	if e.b.as(clientsOnly).scope != "Other" {
		t.Fatal("the limit was lifted by a toggle")
	}

	// A typed group name, a new group, and the names that are not allowed.
	e.press(ownerID, "a:g:56")
	for _, bad := range []string{service.ClusterGroup, strings.Repeat("x", 65), "   "} {
		n = len(e.got())
		e.say(ownerID, bad)
		if text := lastText(e.since(n), "editMessageText"); !strings.Contains(text, "Failed") {
			t.Errorf("group %q accepted: %q", bad, text)
		}
	}
	if e.b.as(noSections).scope != "" {
		t.Fatal("a bad group limited somebody")
	}
	e.say(ownerID, "sales")
	if got := e.b.as(noSections).scope; got != "Sales" && got != "sales" {
		t.Fatalf("scope = %q", got)
	}
	e.press(ownerID, "a:sg:56:#new")
	if p := e.b.pend.get(ownerID); p == nil || p.kind != "ad.grp" {
		t.Fatalf("pending = %+v", p)
	}
	e.say(ownerID, "Brand New")
	if got := e.b.as(noSections).scope; got != "Brand New" {
		t.Fatalf("scope = %q", got)
	}
	if _, scopes, _ := accessSettings(t); !strings.Contains(scopes, "56=Brand New") || strings.Count(scopes, "\n") != 2 {
		t.Fatalf("scopes = %q", scopes)
	}
	// From a group back to sections.
	e.press(ownerID, "a:p:56:cl")
	if as := e.b.as(noSections); as.scope != "" || !as.sections["clients"] {
		t.Fatalf("56 = %+v", as)
	}
	if _, scopes, _ := accessSettings(t); strings.Contains(scopes, "56") {
		t.Fatalf("scopes = %q", scopes)
	}
}

func TestOwnerRemovesAnAdmin(t *testing.T) {
	e := newAccessEnv(t)
	e.configure(map[string]string{"tgBotAdmins": "42, 55"})
	n := len(e.got())
	e.press(ownerID, "a:rm:55")
	msgs := e.since(n)
	if !hasData(keyboardOf(msgs), "a:rmy:55") || !hasData(keyboardOf(msgs), "a:e:55") || !e.b.access().isMember(55) {
		t.Fatalf("confirmation: %v", callbackData(keyboardOf(msgs)))
	}
	e.b.pend.set(clientsOnly, &pending{kind: "cl.search", data: map[string]string{}})
	e.press(ownerID, "a:rmy:55")
	if e.b.access().isMember(55) || e.b.pend.get(clientsOnly) != nil {
		t.Fatal("55 is still an admin")
	}
	admins, scopes, perms := accessSettings(t)
	for _, text := range []string{admins, scopes, perms} {
		if strings.Contains(text, "55") {
			t.Fatalf("55 is still in the settings: %q %q %q", admins, scopes, perms)
		}
	}
	if !strings.Contains(perms, "56=") || !strings.Contains(scopes, "77=Sales") || !strings.Contains(admins, "42") {
		t.Fatalf("the others were touched: %q %q %q", admins, scopes, perms)
	}
	n = len(e.got())
	e.say(clientsOnly, "/clients")
	if msgs := e.since(n); len(msgs) != 1 || msgs[0].Text != e.b.t("denied") {
		t.Fatalf("a removed admin got %+v", msgs)
	}
	n = len(e.got())
	e.press(clientsOnly, "c:ls:a:0")
	if msgs := e.since(n); len(msgs) != 1 || msgs[0].Text != e.b.t("denied") {
		t.Fatalf("a removed admin pressed %+v", msgs)
	}
	// The edit screen of somebody who is gone falls back to the list.
	n = len(e.got())
	e.press(ownerID, "a:e:55")
	if msgs := e.since(n); len(msgs) == 0 || msgs[0].Method != "answerCallbackQuery" || msgs[0].Text == "" {
		t.Fatalf("editing a removed admin: %+v", msgs)
	}
	// Removing a group-limited admin, and one that only a scope line made one.
	e.press(ownerID, "a:rmy:77")
	if e.b.access().isMember(77) {
		t.Fatal("77 is still an admin")
	}
	if _, scopes, _ = accessSettings(t); strings.Contains(scopes, "77") {
		t.Fatalf("scopes = %q", scopes)
	}
}

func TestTheOwnerCannotBeEdited(t *testing.T) {
	e := newAccessEnv(t)
	before := func() string { a, s, p := accessSettings(t); return a + "|" + s + "|" + p }
	snapshot := before()
	for _, data := range []string{"a:e:1000", "a:t:1000:logs", "a:p:1000:none", "a:g:1000", "a:sg:1000:0", "a:rm:1000", "a:rmy:1000",
		"a:q:1000", "a:qa:1000:10", "a:qt:1000", "a:qr:1000", "a:qry:1000", "a:qc:1000", "a:qcy:1000"} {
		n := len(e.got())
		e.press(ownerID, data)
		msgs := e.since(n)
		if len(msgs) != 1 || msgs[0].Method != "answerCallbackQuery" || !strings.Contains(msgs[0].Text, "owner") {
			t.Errorf("%s answered %+v", data, msgs)
		}
	}
	if before() != snapshot {
		t.Fatal("the owner's entry was changed")
	}
	if !e.b.as(ownerID).owner || e.b.as(ownerID).sections != nil {
		t.Fatal("the owner lost access")
	}
	// The same through the document, whatever calls it.
	cfg, _ := loadConfig()
	d := docFromConfig(cfg)
	for name, err := range map[string]error{"remove": d.remove(ownerID), "role": d.setRole(ownerID, role{sections: sectionSet{}}), "add": d.add(ownerID)} {
		if err == nil {
			t.Errorf("%s of the owner succeeded", name)
		}
	}
}

// Rewriting the settings must never widen anybody's access: an unusable line
// becomes an explicit “no access”, and untouched administrators keep their
// role.
func TestEditingKeepsOthersAsTheyWere(t *testing.T) {
	e := newScopeEnv(t)
	e.configure(map[string]string{
		"tgBotOwner":  "1000",
		"tgBotAdmins": "42, 88, 99, 1000",
		"tgBotScopes": "88 Sales\n77=Sales\n99=" + service.ClusterGroup,
		"tgBotPerms":  "55=clients\n56=logs,nonsense\n66",
	})
	acc := e.b.access()
	for _, id := range []int64{88, 99, 66} {
		if r := acc.roleOf(id); r.sections == nil || len(r.sections) != 0 {
			t.Fatalf("%d should start without access: %+v", id, r)
		}
	}
	roles := func() map[int64]string {
		out := map[int64]string{}
		a := e.b.access()
		for _, id := range a.order {
			r := a.roleOf(id)
			out[id] = fmt.Sprintf("%q|%v|%s", r.group, r.sections == nil, keysOf(r.sections))
		}
		return out
	}
	before := roles()

	e.press(ownerID, "a:add")
	e.say(ownerID, "900")
	after := roles()
	for id, want := range before {
		if after[id] != want {
			t.Errorf("%d changed from %s to %s by adding somebody else", id, want, after[id])
		}
	}
	if _, ok := after[900]; !ok {
		t.Fatal("900 was not added")
	}
	// The broken lines are now explicit, and the settings parse cleanly.
	admins, scopes, perms := accessSettings(t)
	cfg, err := loadConfig()
	if err != nil || len(cfg.Locked) != 0 {
		t.Fatalf("locked after rewriting: %v %v", cfg.Locked, err)
	}
	for _, want := range []string{"88=", "99=", "66=", "56=logs", "55=clients"} {
		if !strings.Contains(perms, want) {
			t.Errorf("perms lacks %q: %q", want, perms)
		}
	}
	if scopes != "77=Sales" || !strings.Contains(admins, "42") {
		t.Fatalf("scopes %q admins %q", scopes, admins)
	}
	// And still nobody has more than before.
	for _, id := range []int64{88, 99, 66} {
		n := len(e.got())
		e.say(id, "/clients")
		if msgs := e.since(n); len(msgs) != 1 || msgs[0].Text != e.b.t("noAccess") {
			t.Errorf("%d got %+v", id, msgs)
		}
	}
}

func TestSettingsEditedInTheWebPanelReachTheRunningBot(t *testing.T) {
	e := newAccessEnv(t)
	n := len(e.got())
	e.say(900, "/clients")
	if msgs := e.since(n); len(msgs) != 1 || msgs[0].Text != e.b.t("denied") {
		t.Fatalf("900 before: %+v", msgs)
	}
	e.configure(map[string]string{"tgBotPerms": "900=clients,logs"})
	n = len(e.got())
	e.say(900, "/logs")
	if text := lastText(e.since(n), "sendMessage"); text == e.b.t("noAccess") || text == e.b.t("denied") {
		t.Fatalf("900 after: %q", text)
	}
	// A pending prompt of a section taken away is dropped, not answered.
	n = len(e.got())
	e.say(900, "/add")
	e.b.pend.set(900, &pending{kind: "wiz", key: "name", data: map[string]string{}})
	e.configure(map[string]string{"tgBotPerms": "900=logs"})
	e.say(900, "evil_client")
	if e.exists("evil_client") || e.b.pend.get(900) != nil {
		t.Fatal("a prompt of a revoked section was answered")
	}
}

func TestSupervisorAppliesAccessChangesLive(t *testing.T) {
	e := newScopeEnv(t)
	e.configure(map[string]string{"tgBotEnable": "true", "tgBotToken": "1:abc", "tgBotOwner": "1000", "tgBotAdmins": "42", "tgBotLang": "en"})
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	sv := &supervisor{ctx: context.Background(), configService: e.b.configService}
	defer sv.halt()
	sv.apply(cfg)
	if sv.running == nil {
		t.Fatal("the bot did not start")
	}
	first := sv.running
	waitFor := func(what string, cond func() bool) {
		t.Helper()
		for i := 0; i < 100; i++ {
			if cond() {
				return
			}
			time.Sleep(30 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %s", what)
	}
	menuFor := func(id int64) (call menuCall, found bool) {
		for _, c := range fakeMenuCalls() {
			if c.ChatID == id {
				call, found = c, true
			}
		}
		return call, found
	}
	waitFor("the start-up command menus", func() bool { _, ok := menuFor(42); return ok })
	if !first.access().isMember(1000) || first.access().isMember(55) {
		t.Fatalf("members at start: %v", first.access().order)
	}

	// Only the administrators changed: no restart, and the bot knows at once.
	e.configure(map[string]string{"tgBotPerms": "55=clients"})
	next, _ := loadConfig()
	if next.fingerprint() != cfg.fingerprint() || next.accessKey() == cfg.accessKey() {
		t.Fatal("keys do not separate restarts from live changes")
	}
	sv.apply(next)
	if sv.running != first {
		t.Fatal("a change of administrators restarted the bot")
	}
	if !first.access().isMember(55) || !first.access().roleOf(55).sections["clients"] {
		t.Fatalf("the running bot did not take 55 over: %v", first.access().order)
	}
	waitFor("55's command menu", func() bool {
		c, ok := menuFor(55)
		return ok && c.Method == "setMyCommands" && strings.Contains(strings.Join(c.Commands, " "), "clients") && !strings.Contains(strings.Join(c.Commands, " "), "backup")
	})
	// No new change: nothing is sent again.
	before := len(fakeMenuCalls())
	sv.apply(next)
	time.Sleep(100 * time.Millisecond)
	if got := len(fakeMenuCalls()); got != before {
		t.Fatalf("an unchanged configuration sent %d menus", got-before)
	}

	// An administrator who is taken out loses the menu.
	e.configure(map[string]string{"tgBotPerms": ""})
	cfg2, _ := loadConfig()
	sv.apply(cfg2)
	waitFor("55's menu to be removed", func() bool {
		c, ok := menuFor(55)
		return ok && c.Method == "deleteMyCommands"
	})
	if first.access().isMember(55) {
		t.Fatal("55 is still a member")
	}

	// A new token does restart it, and a disabled bot stops.
	e.configure(map[string]string{"tgBotToken": "2:def"})
	cfg3, _ := loadConfig()
	sv.apply(cfg3)
	if sv.running == nil || sv.running == first {
		t.Fatal("a new token did not restart the bot")
	}
	e.configure(map[string]string{"tgBotEnable": "false"})
	cfg4, _ := loadConfig()
	sv.apply(cfg4)
	if sv.running != nil {
		t.Fatal("the disabled bot is still running")
	}
}

func TestAlertsFollowSections(t *testing.T) {
	e := newAccessEnv(t)
	ctx := context.Background()
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
	for _, id := range []int64{ownerID, fullAdmin, clientsOnly} {
		if text := byChat[id]; !strings.Contains(text, "s1") || !strings.Contains(text, "o1") {
			t.Errorf("%d should get every client alert:\n%s", id, text)
		}
	}
	if text := byChat[salesAdmin]; !strings.Contains(text, "s1") || strings.Contains(text, "o1") {
		t.Errorf("the limited admin got:\n%s", text)
	}
	for _, id := range []int64{noSections, nodesOnly, coreOnly} {
		if text := byChat[id]; text != "" {
			t.Errorf("%d got client alerts without the Clients section:\n%s", id, text)
		}
	}

	recipients := func(sections ...string) string {
		n := len(e.got())
		e.b.broadcast(ctx, "notice", sections...)
		var ids []string
		for _, m := range e.since(n) {
			ids = append(ids, strconv.FormatInt(m.ChatID, 10))
		}
		return strings.Join(ids, " ")
	}
	if got := recipients("nodes"); got != "1000 42 57" {
		t.Errorf("node alerts go to %q", got)
	}
	if got := recipients("home", "core"); got != "1000 42 58" {
		t.Errorf("core alerts go to %q", got)
	}
	if got := recipients("backup"); got != "1000 42" {
		t.Errorf("backups go to %q", got)
	}
	n := len(e.got())
	e.b.announce(ctx)
	if got := len(e.since(n)); got != len(e.b.access().order) {
		t.Errorf("the start-up notice reached %d of %d admins", got, len(e.b.access().order))
	}
}

func TestScheduledReportGoesToTheSectionsItCovers(t *testing.T) {
	e := newAccessEnv(t)
	e.configure(map[string]string{"tgBotPerms": "55=stats\n56=\n57=home\n58=backup"})
	n := len(e.got())
	e.b.sendReport(context.Background())
	got := map[int64]string{}
	for _, m := range e.since(n) {
		got[m.ChatID] += m.Text + "\n"
	}
	title := e.b.t("reportTitle")
	for _, id := range []int64{ownerID, fullAdmin, clientsOnly, nodesOnly} {
		if !strings.Contains(got[id], title) {
			t.Errorf("%d got no report", id)
		}
	}
	for _, id := range []int64{salesAdmin, noSections, coreOnly} {
		if got[id] != "" {
			t.Errorf("%d got a report:\n%s", id, got[id])
		}
	}
	// The parts differ: stats only gets the traffic, home only the status.
	if got[clientsOnly] == got[nodesOnly] || got[clientsOnly] == got[fullAdmin] || got[nodesOnly] == got[fullAdmin] {
		t.Error("the reports are not split by section")
	}
	if !strings.Contains(got[fullAdmin], e.b.trafficText()) {
		t.Error("the full report lost the traffic part")
	}
}

func TestAdminNamesAreLookedUpOnceAndFailuresAreQuiet(t *testing.T) {
	e := newAccessEnv(t)
	fakeChat(clientsOnly, "Ali")
	names := e.b.adminNames(context.Background(), []int64{clientsOnly, noSections})
	if names[clientsOnly] != "Ali" || names[noSections] != "" {
		t.Fatalf("names = %v", names)
	}
	// Cached: the person can be renamed in Telegram without the screen changing
	// before the cache runs out.
	fakeChat(clientsOnly, "Changed")
	if got := e.b.adminNames(context.Background(), []int64{clientsOnly})[clientsOnly]; got != "Ali" {
		t.Fatalf("name = %q, want the cached one", got)
	}
	e.b.acc.nameMu.Lock()
	e.b.acc.names[clientsOnly] = nameEntry{name: "Ali", at: time.Now().Add(-2 * nameTTL)}
	e.b.acc.nameMu.Unlock()
	if got := e.b.adminNames(context.Background(), []int64{clientsOnly})[clientsOnly]; got != "Changed" {
		t.Fatalf("name = %q after the cache expired", got)
	}
	// HTML in a name is escaped on the screen.
	fakeChat(noSections, "<b>x</b> & co")
	e.b.acc.nameMu.Lock()
	delete(e.b.acc.names, noSections)
	e.b.acc.nameMu.Unlock()
	n := len(e.got())
	e.press(ownerID, "a:ls")
	if text := lastText(e.since(n), "editMessageText"); strings.Contains(text, "<b>x</b>") || !strings.Contains(text, "&lt;b&gt;x&lt;/b&gt;") {
		t.Fatalf("a name was not escaped:\n%s", text)
	}
}

func TestCommandMenusFollowTheRights(t *testing.T) {
	e := newAccessEnv(t)
	ctx := context.Background()
	e.b.registerCommands(ctx)
	menus := map[int64]menuCall{}
	for _, c := range fakeMenuCalls() {
		menus[c.ChatID] = c
	}
	for _, id := range e.b.access().order {
		if c, ok := menus[id]; !ok || c.Method != "setMyCommands" || len(c.Commands) == 0 {
			t.Errorf("no menu for %d: %+v", id, c)
		}
	}
	if !strings.Contains(strings.Join(menus[fullAdmin].Commands, " "), "restart") || strings.Contains(strings.Join(menus[clientsOnly].Commands, " "), "restart") {
		t.Errorf("menus: %v / %v", menus[fullAdmin].Commands, menus[clientsOnly].Commands)
	}
	// The owner's edits update the menu of that admin only.
	before := len(fakeMenuCalls())
	e.press(ownerID, "a:t:55:backup")
	calls := fakeMenuCalls()[before:]
	if len(calls) != 1 || calls[0].ChatID != clientsOnly || !strings.Contains(strings.Join(calls[0].Commands, " "), "backup") {
		t.Fatalf("menu calls after a toggle: %+v", calls)
	}
	before = len(fakeMenuCalls())
	e.press(ownerID, "a:rmy:55")
	calls = fakeMenuCalls()[before:]
	if len(calls) != 1 || calls[0].Method != "deleteMyCommands" || calls[0].ChatID != clientsOnly {
		t.Fatalf("menu calls after a removal: %+v", calls)
	}
}

// A callback whose chat is not private or whose data is junk must not reach the
// owner's handlers.
func TestAdminsCallbacksIgnoreJunk(t *testing.T) {
	e := newAccessEnv(t)
	a, s, p := accessSettings(t)
	for _, data := range []string{"a:e", "a:e:abc", "a:t:55", "a:t:55:nonsense", "a:p:55:weird", "a:sg:55:99", "a:sg:55:x", "a:rm:99999", "a:rmy:99999", "a:zzz:55", "a:g:99999", "a:q", "a:q:abc", "a:q:99999", "a:qa:55", "a:qa:55:x", "a:qa:55:-5", "a:qt:99999", "a:qry:99999", "a:qcy:99999"} {
		e.press(ownerID, data)
	}
	if a2, s2, p2 := accessSettings(t); a != a2 || s != s2 || p != p2 {
		t.Fatalf("junk buttons changed the settings: %q %q %q", a2, s2, p2)
	}
	if qs := allQuotas(); len(qs) != 0 {
		t.Fatalf("junk buttons made volume limits: %+v", qs)
	}
	// The group chooser uses an index; a stale or invented one is refused.
	n := len(e.got())
	e.press(ownerID, "a:sg:55:99")
	if msgs := e.since(n); len(msgs) != 1 || msgs[0].Method != "answerCallbackQuery" {
		t.Fatalf("a stale group index answered %+v", msgs)
	}
}
