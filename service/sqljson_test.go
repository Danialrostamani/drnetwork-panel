package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// A client's inbounds stored as exactly six bytes -- [1,10], [12,3] -- pass
// SQLite's quick JSONB check, so before the CAST every query reading them
// failed with "malformed JSON": no inbound's users could be built, and with
// them neither the core config nor any client save.
func TestSixByteInboundsList(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "jsonb.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	for _, id := range []uint{1, 10} {
		in := model.Inbound{Id: id, Tag: fmt.Sprintf("in-%d", id), Type: "vless", Addrs: json.RawMessage("[]"), OutJson: json.RawMessage("{}"), Options: json.RawMessage(`{"listen_port":443}`)}
		if err := db.Create(&in).Error; err != nil {
			t.Fatal(err)
		}
	}
	for name, inbounds := range map[string]string{"ali": "[1,10]", "sara": "[10]", "reza": "[1, 2]"} {
		c := model.Client{Name: name, Enable: true, Inbounds: json.RawMessage(inbounds), Links: json.RawMessage("[]"),
			Config: json.RawMessage(`{"vless":{"name":"` + name + `","uuid":"11111111-1111-1111-1111-111111111111"}}`)}
		if err := db.Create(&c).Error; err != nil {
			t.Fatal(err)
		}
	}

	s := &InboundService{}
	out, err := s.addUsers(db, []byte(`{"type":"vless","tag":"in-10","listen_port":443}`), 10, "vless")
	if err != nil {
		t.Fatal("inbound users: ", err)
	}
	var withUsers struct {
		Users []struct{ Name string } `json:"users"`
	}
	if err := json.Unmarshal(out, &withUsers); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, u := range withUsers.Users {
		got = append(got, u.Name)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, []string{"ali", "sara"}) {
		t.Fatalf("users of in-10 = %v, want [ali sara]", got)
	}

	keep, err := s.enabledClientNames(db, 1)
	if err != nil {
		t.Fatal("enabled names: ", err)
	}
	if _, ok := keep["ali"]; !ok || len(keep) != 2 {
		t.Fatalf("enabled on in-1 = %v, want ali and reza", keep)
	}

	all, err := s.GetAll()
	if err != nil {
		t.Fatal("inbound list: ", err)
	}
	for _, in := range *all {
		if users, _ := in["users"].([]string); in["tag"] == "in-10" && len(users) != 2 {
			t.Fatalf("in-10 lists users %v", in["users"])
		}
	}

	if err := (&ClientService{}).UpdateClientsOnInboundDelete(db, 10, "in-10"); err != nil {
		t.Fatal("inbound delete: ", err)
	}
	var ali model.Client
	if err := db.Where("name = ?", "ali").First(&ali).Error; err != nil {
		t.Fatal(err)
	}
	var ids []uint
	if err := json.Unmarshal(ali.Inbounds, &ids); err != nil || !reflect.DeepEqual(ids, []uint{1}) {
		t.Fatalf("ali's inbounds after deleting in-10 = %s (%v)", ali.Inbounds, err)
	}
}

// sqlJSONCall finds a SQLite JSON function in Go source.
var sqlJSONCall = regexp.MustCompile(`\bjson_(each|tree|extract|array_length|type|valid|set|insert|replace|remove|patch)\(`)

// TestSQLJSONTakesText keeps new SQL from handing a JSON column, a BLOB,
// straight to a JSON function: see TestSixByteInboundsList.
func TestSQLJSONTakesText(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "frontend", "node_modules", "web":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, loc := range sqlJSONCall.FindAllIndex(src, -1) {
			found++
			if !strings.HasPrefix(string(src[loc[1]:]), "CAST(") {
				line := strings.Count(string(src[:loc[0]]), "\n") + 1
				t.Errorf("%s:%d: %s must take CAST(column AS TEXT)", strings.TrimPrefix(path, root+"/"), line, src[loc[0]:loc[1]])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found == 0 {
		t.Fatal("found no SQL JSON function at all; is the walk rooted at the repository?")
	}
}
