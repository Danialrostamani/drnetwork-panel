package service

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/core"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"

	"gorm.io/gorm"
)

// attachFixture builds a master with five inbounds that take clients (one of
// them hosted on a node), four that do not, and five clients in different
// states. It returns the ids of the inbounds that take clients, ascending.
func attachFixture(t *testing.T) (*gorm.DB, []uint) {
	t.Helper()
	if err := database.InitDB(filepath.Join(t.TempDir(), "attach-all.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()

	node := uint(3)
	inbound := func(tag, typ, options string, nodeID *uint) uint {
		in := model.Inbound{Tag: tag, Type: typ, NodeId: nodeID, Addrs: json.RawMessage("[]"), OutJson: json.RawMessage("{}"), Options: json.RawMessage(options)}
		if err := db.Create(&in).Error; err != nil {
			t.Fatal(err)
		}
		return in.Id
	}
	eligible := []uint{
		inbound("in-vless", "vless", `{"listen":"::","listen_port":443}`, nil),
		inbound("in-trojan", "trojan", `{"listen":"::","listen_port":8443}`, nil),
		inbound("in-ss", "shadowsocks", `{"listen_port":8388,"method":"aes-128-gcm"}`, nil),
		inbound("in-stls3", "shadowtls", `{"listen_port":9443,"version":3}`, nil),
		inbound("remote-vless", "vless", `{"listen_port":443}`, &node),
	}
	// None of these takes clients, so none must be attached.
	inbound("in-direct", "direct", `{"listen_port":53}`, nil)
	inbound("in-ss-managed", "shadowsocks", `{"listen_port":8389,"managed":true,"method":"2022-blake3-aes-128-gcm"}`, nil)
	inbound("in-stls2", "shadowtls", `{"listen_port":9444,"version":2}`, nil)
	inbound("", "vless", `{"listen_port":9445}`, nil)

	config := func(name string) json.RawMessage {
		return json.RawMessage(`{"vless":{"name":"` + name + `","uuid":"11111111-1111-1111-1111-111111111111"},"trojan":{"name":"` + name + `","password":"secret"}}`)
	}
	client := func(name, group string, enable bool, inbounds json.RawMessage) {
		c := model.Client{
			Name: name, Group: group, Enable: enable, Config: config(name), Inbounds: inbounds, Links: json.RawMessage("[]"),
			Up: 111, Down: 222, TotalUp: 5, Volume: 1 << 30, Desc: "keep", TgId: 99,
			AutoReset: true, ResetDays: 7, DelayStart: true,
		}
		if err := db.Create(&c).Error; err != nil {
			t.Fatal(err)
		}
	}
	encode := func(ids ...uint) json.RawMessage { raw, _ := json.Marshal(ids); return raw }
	client("alice", "vip", true, encode(eligible[0])) // has one of them
	client("bob", "", true, encode())                 // has none
	client("carol", "", true, encode(eligible...))    // has all of them already
	client("dave", "team", false, nil)                // disabled, no inbound list at all
	client("pushed", clusterGroup, true, encode())    // belongs to the master that pushed it
	return db, eligible
}

func clientInbounds(t *testing.T, db *gorm.DB, name string) []uint {
	t.Helper()
	var c model.Client
	if err := db.Where("name = ?", name).First(&c).Error; err != nil {
		t.Fatalf("client %s: %v", name, err)
	}
	var ids []uint
	if len(c.Inbounds) > 0 {
		if err := json.Unmarshal(c.Inbounds, &ids); err != nil {
			t.Fatalf("client %s inbounds %q: %v", name, c.Inbounds, err)
		}
	}
	return ids
}

func TestAttachAllInboundsToAllClients(t *testing.T) {
	db, eligible := attachFixture(t)
	want := append([]uint(nil), eligible...)
	sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })

	// The preview counts alice, bob and dave: carol has everything and the
	// pushed client is not ours to change.
	if n, m, err := (&ClientService{}).AttachAllPreview(); err != nil || n != 3 || m != len(want) {
		t.Fatalf("preview = %d clients, %d inbounds, err %v", n, m, err)
	}
	touched, err := (&ClientService{}).Save(db, "attachall", json.RawMessage(`{}`), "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(touched, want) {
		t.Fatalf("inbounds reported as changed = %v, want %v", touched, want)
	}
	if n, _, err := (&ClientService{}).AttachAllPreview(); err != nil || n != 0 {
		t.Fatalf("preview after attaching = %d clients, err %v", n, err)
	}
	for _, name := range []string{"alice", "bob", "carol", "dave"} {
		if got := clientInbounds(t, db, name); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s inbounds = %v, want %v", name, got, want)
		}
	}
	if got := clientInbounds(t, db, "pushed"); len(got) != 0 {
		t.Fatalf("a client pushed by the master was changed: %v", got)
	}

	// Only the inbound list and the links were written.
	var alice model.Client
	if err := db.Where("name = ?", "alice").First(&alice).Error; err != nil {
		t.Fatal(err)
	}
	if !alice.Enable || alice.Group != "vip" || alice.Desc != "keep" || alice.Volume != 1<<30 ||
		alice.Up != 111 || alice.Down != 222 || alice.TotalUp != 5 || alice.TgId != 99 ||
		!alice.AutoReset || alice.ResetDays != 7 || !alice.DelayStart {
		t.Fatalf("attaching changed other fields of the client: %+v", alice)
	}
	var dave model.Client
	if err := db.Where("name = ?", "dave").First(&dave).Error; err != nil {
		t.Fatal(err)
	}
	if dave.Enable {
		t.Fatal("a disabled client was enabled")
	}
	// The links of the local inbounds follow; a node's links come from the sync.
	var links []map[string]string
	if err := json.Unmarshal(alice.Links, &links); err != nil {
		t.Fatal(err)
	}
	remarks := map[string]bool{}
	for _, link := range links {
		if link["type"] == "local" {
			remarks[link["remark"]] = true
		}
	}
	if !remarks["in-vless"] || !remarks["in-trojan"] || remarks["remote-vless"] {
		t.Fatalf("links after attaching = %v", links)
	}

	// Asking again changes nothing.
	again, err := (&ClientService{}).Save(db, "attachall", json.RawMessage(`{}`), "example.com")
	if err != nil || len(again) != 0 {
		t.Fatalf("second run: changed=%v err=%v", again, err)
	}
}

func TestAttachAllThroughConfigServiceReportsInboundsAndLogsIt(t *testing.T) {
	db, _ := attachFixture(t)
	if err := (&SettingService{}).SetMaintenance(true); err != nil {
		t.Fatal(err)
	}
	if _, err := (&SettingService{}).GetAllSetting(); err != nil {
		t.Fatal(err)
	}
	previous := corePtr
	t.Cleanup(func() {
		time.Sleep(200 * time.Millisecond)
		corePtr = previous
	})
	cs := NewConfigService(core.NewCore())

	objs, err := cs.Save("clients", "attachall", json.RawMessage(`{}`), "", "tester", "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(objs, []string{"clients", "inbounds"}) {
		t.Fatalf("objects to reload = %v", objs)
	}
	var logged int64
	db.Model(model.Changes{}).Where("`key` = ? AND action = ? AND actor = ?", "clients", "attachall", "tester").Count(&logged)
	if logged != 1 {
		t.Fatalf("change log entries = %d", logged)
	}
	// Nothing left to add: only the clients are reloaded.
	objs, err = cs.Save("clients", "attachall", json.RawMessage(`{}`), "", "tester", "example.com")
	if err != nil || !reflect.DeepEqual(objs, []string{"clients"}) {
		t.Fatalf("second save: objs=%v err=%v", objs, err)
	}
}

// The inbound list tells the panel which inbounds take clients by giving them a
// "users" field, and the attach-all action must pick exactly those.
func TestInboundListAndAttachAllAgreeOnWhichInboundsTakeClients(t *testing.T) {
	db, eligible := attachFixture(t)
	listed, err := (&InboundService{}).GetAll()
	if err != nil {
		t.Fatal(err)
	}
	var withUsers []uint
	for _, row := range *listed {
		if _, ok := row["users"]; ok {
			tag, _ := row["tag"].(string)
			if tag != "" {
				withUsers = append(withUsers, row["id"].(uint))
			}
		}
	}
	sort.Slice(withUsers, func(i, j int) bool { return withUsers[i] < withUsers[j] })
	want := append([]uint(nil), eligible...)
	sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
	if !reflect.DeepEqual(withUsers, want) {
		t.Fatalf("inbounds with a users field = %v, want %v", withUsers, want)
	}
	touched, err := (&ClientService{}).Save(db, "attachall", json.RawMessage(`{}`), "example.com")
	if err != nil || !reflect.DeepEqual(touched, want) {
		t.Fatalf("attach-all picked %v (err %v), want %v", touched, err, want)
	}
}
