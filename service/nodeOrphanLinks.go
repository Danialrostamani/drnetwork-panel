package service

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"

	"gorm.io/gorm"
)

// A node's links in a client are named "[node] tag". Deleting a node used to
// leave those links in every client whose inbound had already gone, and as
// the node no longer existed nothing recognised them as node links again:
// the subscriptions kept showing a server that was long gone. Deleting a node
// now takes its links with it, and the links older deletions left behind can
// be found and removed here.

var nodeRemarkRe = regexp.MustCompile(`^\[([^\]]+)\] `)

// OrphanNodeLink is a node name client links still carry with no such node.
type OrphanNodeLink struct {
	Name    string `json:"name"`
	Clients int    `json:"clients"`
	Links   int    `json:"links"`
}

func orphanName(link map[string]string, nodes map[string]bool) string {
	if link["type"] == "local" {
		return ""
	}
	m := nodeRemarkRe.FindStringSubmatch(link["remark"])
	if m == nil || nodes[m[1]] {
		return ""
	}
	return m[1]
}

func currentNodeNames(tx *gorm.DB) (map[string]bool, error) {
	var names []string
	if err := tx.Model(model.Node{}).Pluck("name", &names).Error; err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(names))
	for _, n := range names {
		out[n] = true
	}
	return out, nil
}

// OrphanNodeLinks lists them, most clients first.
func OrphanNodeLinks() ([]OrphanNodeLink, error) {
	db := database.GetDB()
	nodes, err := currentNodeNames(db)
	if err != nil {
		return nil, err
	}
	var clients []model.Client
	if err := db.Select("id", "links").Find(&clients).Error; err != nil {
		return nil, err
	}
	by := map[string]*OrphanNodeLink{}
	for _, c := range clients {
		var links []map[string]string
		if json.Unmarshal(c.Links, &links) != nil {
			continue
		}
		seen := map[string]bool{}
		for _, l := range links {
			name := orphanName(l, nodes)
			if name == "" {
				continue
			}
			o := by[name]
			if o == nil {
				o = &OrphanNodeLink{Name: name}
				by[name] = o
			}
			o.Links++
			if !seen[name] {
				seen[name] = true
				o.Clients++
			}
		}
	}
	out := make([]OrphanNodeLink, 0, len(by))
	for _, o := range by {
		out = append(out, *o)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Clients != out[j].Clients {
			return out[i].Clients > out[j].Clients
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// RemoveOrphanNodeLinks drops the links of those names from every client; a
// name that is a node again is left alone. It returns the clients changed.
func RemoveOrphanNodeLinks(names []string, actor string) (int, error) {
	want := map[string]bool{}
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" {
			want[n] = true
		}
	}
	if len(want) == 0 {
		return 0, nil
	}
	changed := 0
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		nodes, err := currentNodeNames(tx)
		if err != nil {
			return err
		}
		n, err := dropClientLinks(tx, func(l map[string]string) bool {
			name := orphanName(l, nodes)
			return name != "" && want[name]
		})
		if err != nil {
			return err
		}
		changed = n
		if n > 0 {
			obj, _ := json.Marshal(map[string]any{"orphanNodeLinks": names, "clients": n})
			return tx.Create(&model.Changes{DateTime: time.Now().Unix(), Actor: actor, Key: "clients", Action: "edit", Obj: obj}).Error
		}
		return nil
	})
	if err == nil && changed > 0 {
		LastUpdate = time.Now().Unix()
	}
	return changed, err
}

// dropNodeLinks takes a deleted node's links out of every client.
func dropNodeLinks(tx *gorm.DB, nodeName string) error {
	if nodeName == "" {
		return nil
	}
	prefix := nodeLinkPrefix(nodeName)
	n, err := dropClientLinks(tx, func(l map[string]string) bool {
		return l["type"] != "local" && strings.HasPrefix(l["remark"], prefix)
	})
	if err == nil && n > 0 {
		LastUpdate = time.Now().Unix()
	}
	return err
}

func dropClientLinks(tx *gorm.DB, drop func(map[string]string) bool) (int, error) {
	var clients []model.Client
	if err := tx.Select("id", "links").Find(&clients).Error; err != nil {
		return 0, err
	}
	changed := 0
	for _, c := range clients {
		var links []map[string]string
		if json.Unmarshal(c.Links, &links) != nil {
			continue
		}
		kept := make([]map[string]string, 0, len(links))
		for _, l := range links {
			if !drop(l) {
				kept = append(kept, l)
			}
		}
		if len(kept) == len(links) {
			continue
		}
		raw, err := json.MarshalIndent(kept, "", "  ")
		if err != nil {
			return changed, err
		}
		if err := tx.Model(model.Client{}).Where("id = ?", c.Id).Update("links", raw).Error; err != nil {
			return changed, err
		}
		changed++
	}
	return changed, nil
}
