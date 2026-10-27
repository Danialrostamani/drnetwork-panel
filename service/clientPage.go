package service

import (
	"encoding/json"
	"unicode/utf8"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// ClientLocation is what a client used in its current cycle on one server,
// named the way its user knows it.
type ClientLocation struct {
	Node uint   `json:"-"`
	Name string `json:"tag"`
	Up   int64  `json:"up"`
	Down int64  `json:"down"`
}

// LocationName names a server by the shortest of its inbound tags: the
// variants ("FR-2", "France 2 . V6") carry the base name plus a suffix.
func LocationName(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	name := tags[0]
	for _, t := range tags[1:] {
		if utf8.RuneCountInString(t) < utf8.RuneCountInString(name) {
			name = t
		}
	}
	return name
}

// ClientLocations are the servers a client is on, in the order of its
// inbounds, each with what the client used there (ClientLocationUsage).
func (s *StatsService) ClientLocations(c model.Client) []ClientLocation {
	var ids []uint
	_ = json.Unmarshal(c.Inbounds, &ids)
	if len(ids) == 0 {
		return nil
	}
	var inbounds []model.Inbound
	_ = database.GetDB().Select("id", "tag", "node_id").Where("id IN ?", ids).Find(&inbounds).Error
	byID := make(map[uint]model.Inbound, len(inbounds))
	for _, in := range inbounds {
		byID[in.Id] = in
	}
	var order []uint
	tags := map[uint][]string{}
	for _, id := range ids {
		in, ok := byID[id]
		if !ok {
			continue
		}
		node := uint(0)
		if in.NodeId != nil {
			node = *in.NodeId
		}
		if _, seen := tags[node]; !seen {
			order = append(order, node)
		}
		tags[node] = append(tags[node], in.Tag)
	}
	if len(order) == 0 {
		return nil
	}
	usage := s.ClientLocationUsage(c)
	out := make([]ClientLocation, 0, len(order))
	for _, node := range order {
		u := usage[node]
		out = append(out, ClientLocation{Node: node, Name: LocationName(tags[node]), Up: u.Up, Down: u.Down})
	}
	return out
}

// ClientRecentUsage is what a client used in the last day and the last week,
// from the traffic history of the master and of its nodes; ok is false when
// there is no history to tell.
func (s *StatsService) ClientRecentUsage(c model.Client, now int64) (day, week int64, ok bool) {
	for _, x := range s.locationSamples(c, now-7*86400, now) {
		v := x.up + x.down
		week += v
		if x.at >= now-86400 {
			day += v
		}
		ok = true
	}
	return day, week, ok
}

// IsClientOnline tells whether a client has a live connection on the master
// or on any of its nodes.
func (s *StatsService) IsClientOnline(name string) bool {
	on, err := s.GetClusterOnlines()
	if err != nil {
		return false
	}
	for _, u := range on.User {
		if u == name {
			return true
		}
	}
	return false
}
