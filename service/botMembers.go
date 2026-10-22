package service

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/util/common"
)

// BotMember is one person who used the Telegram bot's shop, as the panel's
// members page shows them: who they are, their introducer, whether they took
// the test account and what they bought.
type BotMember struct {
	TgId      int64    `json:"tgId"`
	Name      string   `json:"name"`
	Username  string   `json:"username"`
	Blocked   bool     `json:"blocked"`
	AdminId   int64    `json:"adminId"`
	Note      string   `json:"note"`
	FirstSeen int64    `json:"firstSeen"`
	LastSeen  int64    `json:"lastSeen"`
	Ref       string   `json:"ref"`
	RefGroup  string   `json:"refGroup"`
	TrialAt   int64    `json:"trialAt"`
	Buys      int      `json:"buys"`
	Renews    int      `json:"renews"`
	Spent     int64    `json:"spent"`
	LastBuy   int64    `json:"lastBuy"`
	Pending   int      `json:"pending"`
	Balance   int64    `json:"balance"`
	Clients   []string `json:"clients"`
	ClientIds []uint   `json:"clientIds"`
	// TrialClients are the clients made by their test accounts.
	TrialClients []uint `json:"trialClients"`
}

// BotRefOption is an introducer word set in the bot's shop.
type BotRefOption struct {
	Word  string `json:"word"`
	Group string `json:"group"`
}

type BotMemberService struct{}

// Refs are the introducer words of the shop, in the order the owner made them.
func (s *BotMemberService) Refs() []BotRefOption {
	raw, err := (&SettingService{}).GetTgBotShop()
	if err != nil {
		return nil
	}
	var shop struct {
		Refs []BotRefOption `json:"refs"`
	}
	if json.Unmarshal([]byte(raw), &shop) != nil {
		return nil
	}
	out := make([]BotRefOption, 0, len(shop.Refs))
	for _, r := range shop.Refs {
		if strings.TrimSpace(r.Word) != "" {
			out = append(out, r)
		}
	}
	return out
}

// List is every bot member, newest first. Somebody known only from an order,
// a wallet or an introducer (from before customers were recorded) is listed
// too.
func (s *BotMemberService) List() ([]BotMember, error) {
	db := database.GetDB()
	members := map[int64]*BotMember{}
	get := func(id int64) *BotMember {
		m := members[id]
		if m == nil {
			m = &BotMember{TgId: id, Clients: []string{}, ClientIds: []uint{}, TrialClients: []uint{}}
			members[id] = m
		}
		return m
	}

	var customers []model.BotCustomer
	if err := db.Find(&customers).Error; err != nil {
		return nil, err
	}
	for _, c := range customers {
		if c.TgId == 0 {
			continue
		}
		m := get(c.TgId)
		m.Name, m.Username, m.Blocked, m.Note, m.AdminId = c.Name, c.Username, c.Blocked, c.Note, c.AdminId
		m.FirstSeen, m.LastSeen = c.FirstSeen, c.LastSeen
	}

	var orders []model.BotOrder
	if err := db.Select("tg_id", "topic", "status", "amount", "client_id", "created_at", "updated_at").Find(&orders).Error; err != nil {
		return nil, err
	}
	for _, o := range orders {
		if o.TgId == 0 {
			continue
		}
		m := get(o.TgId)
		if m.FirstSeen == 0 || (o.CreatedAt > 0 && o.CreatedAt < m.FirstSeen) {
			m.FirstSeen = o.CreatedAt
		}
		if o.Status == "pending" {
			m.Pending++
			continue
		}
		if o.Status != "approved" {
			continue
		}
		at := o.UpdatedAt
		if at == 0 {
			at = o.CreatedAt
		}
		switch o.Topic {
		case "test":
			if m.TrialAt == 0 || at < m.TrialAt {
				m.TrialAt = at
			}
			if o.ClientId != 0 {
				m.TrialClients = append(m.TrialClients, o.ClientId)
			}
		case "buy", "renew":
			if o.Topic == "buy" {
				m.Buys++
			} else {
				m.Renews++
			}
			m.Spent += o.Amount
			if at > m.LastBuy {
				m.LastBuy = at
			}
		}
	}

	var refs []model.BotRef
	if err := db.Find(&refs).Error; err != nil {
		return nil, err
	}
	for _, r := range refs {
		if r.TgId == 0 {
			continue
		}
		m := get(r.TgId)
		m.Ref, m.RefGroup = r.Word, r.Group
	}

	var wallets []model.BotWallet
	if err := db.Find(&wallets).Error; err != nil {
		return nil, err
	}
	for _, w := range wallets {
		if w.TgId == 0 {
			continue
		}
		get(w.TgId).Balance = w.Balance
	}

	var clients []model.Client
	if err := db.Select("id", "name", "tg_id").Where("tg_id <> 0").Order("id").Find(&clients).Error; err != nil {
		return nil, err
	}
	for _, c := range clients {
		if m := members[c.TgId]; m != nil {
			m.Clients = append(m.Clients, c.Name)
			m.ClientIds = append(m.ClientIds, c.Id)
		}
	}

	out := make([]BotMember, 0, len(members))
	for _, m := range members {
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FirstSeen != out[j].FirstSeen {
			return out[i].FirstSeen > out[j].FirstSeen
		}
		return out[i].TgId < out[j].TgId
	})
	return out, nil
}

// BotMemberClient is a client a member can be given in the panel.
type BotMemberClient struct {
	Id   uint   `json:"id"`
	Name string `json:"name"`
	TgId int64  `json:"tgId"`
}

// Clients are every client of the panel, by name, with whom they are bound to.
func (s *BotMemberService) Clients() []BotMemberClient {
	out := []BotMemberClient{}
	var rows []model.Client
	if database.GetDB().Select("id", "name", "tg_id").Order("name").Find(&rows).Error != nil {
		return out
	}
	for _, c := range rows {
		out = append(out, BotMemberClient{Id: c.Id, Name: c.Name, TgId: c.TgId})
	}
	return out
}

// SetRef gives a member the introducer with this word, as if they had sent
// it to the bot; an empty word removes their introducer.
func (s *BotMemberService) SetRef(tgID int64, word string) error {
	if tgID == 0 {
		return common.NewError("bad telegram id")
	}
	db := database.GetDB()
	want := strings.ToLower(strings.TrimSpace(word))
	if want == "" {
		return db.Where("tg_id = ?", tgID).Delete(&model.BotRef{}).Error
	}
	for _, r := range s.Refs() {
		if strings.ToLower(strings.TrimSpace(r.Word)) == want {
			return db.Save(&model.BotRef{TgId: tgID, Word: r.Word, Group: r.Group}).Error
		}
	}
	return common.NewError("unknown introducer: ", word)
}
