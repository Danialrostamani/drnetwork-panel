package api

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
	"github.com/Danialrostamani/drnetwork-panel/util/common"

	"github.com/gin-gonic/gin"
)

// The Sales page: the shop's numbers, plans, codes, orders and settings.

func (a *ApiService) GetShop(c *gin.Context) {
	s := &service.ShopService{}
	days, _ := strconv.Atoi(c.Query("days"))
	loc, _ := (&service.SettingService{}).GetTimeLocation()
	stats, err := s.Stats(loc, days)
	if err != nil {
		jsonMsg(c, "shop", err)
		return
	}
	plans, _ := s.Plans(false)
	codes, _ := s.Discounts()
	orders, _ := s.Orders(strings.TrimSpace(c.Query("status")), 0, 200)
	sms, _ := s.SmsLog(100)
	tickets, _ := s.Tickets(0, true, 80)
	users, _ := s.ListUsers(80)
	jsonObj(c, map[string]any{
		"sms":         sms,
		"stats":       stats,
		"plans":       plans,
		"discounts":   codes,
		"orders":      orders,
		"settings":    s.RawSettings(),
		"tickets":     tickets,
		"users":       users,
		"groups":      s.ClientGroups(),
		"nodes":       s.NodeNames(),
		"openTickets": s.OpenTickets(),
	}, nil)
}

func formID(c *gin.Context, key string) (int64, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(c.PostForm(key)), 10, 64)
	if err != nil || n <= 0 {
		return 0, common.NewError("invalid ", key)
	}
	return n, nil
}

func (a *ApiService) SaveShop(c *gin.Context) {
	s := &service.ShopService{}
	data := []byte(c.PostForm("data"))
	var err error
	switch c.PostForm("obj") {
	case "plan":
		var p model.ShopPlan
		if err = json.Unmarshal(data, &p); err == nil {
			err = s.SavePlan(&p)
		}
	case "planDel":
		var id int64
		if id, err = formID(c, "id"); err == nil {
			err = s.DeletePlan(uint(id))
		}
	case "discount":
		var d model.ShopDiscount
		if err = json.Unmarshal(data, &d); err == nil {
			err = s.SaveDiscount(&d)
		}
	case "discountDel":
		var id int64
		if id, err = formID(c, "id"); err == nil {
			err = s.DeleteDiscount(uint(id))
		}
	case "setting":
		var m map[string]string
		if err = json.Unmarshal(data, &m); err == nil {
			for k, v := range m {
				if err = s.SetSetting(k, v); err != nil {
					break
				}
			}
		}
	case "wallet":
		var id, amount int64
		if id, err = formID(c, "tgId"); err == nil {
			amount, err = strconv.ParseInt(strings.TrimSpace(c.PostForm("amount")), 10, 64)
			if err == nil {
				note := strings.TrimSpace(c.PostForm("note"))
				if note == "" {
					note = "admin"
				}
				_, err = s.AddFunds(nil, id, amount, note, 0, 0)
			}
		}
	case "reseller":
		var id int64
		if id, err = formID(c, "tgId"); err == nil {
			var pct int
			if pct, err = strconv.Atoi(strings.TrimSpace(c.PostForm("percent"))); err == nil {
				err = s.SetResellerPercent(id, pct)
			}
		}
	case "block":
		var id int64
		if id, err = formID(c, "tgId"); err == nil {
			err = s.SetBlocked(id, c.PostForm("blocked") == "true")
		}
	case "smsTest":
		text := c.PostForm("text")
		if len(text) > 8000 {
			text = text[:8000]
		}
		amount, ok, why := service.ParseDepositSms(text, s.Settings().SmsRial)
		jsonObj(c, map[string]any{"amount": amount, "ok": ok, "why": why}, nil)
		return
	case "decide":
		var id int64
		if id, err = formID(c, "id"); err == nil {
			if service.ShopDecider == nil {
				err = service.ErrBotDown
			} else {
				var msg string
				msg, err = service.ShopDecider(uint(id), c.PostForm("approve") == "true")
				if err == nil {
					jsonMsgObj(c, "save", msg, nil)
					return
				}
			}
		}
	case "users":
		q := strings.TrimSpace(c.PostForm("query"))
		var list []model.ShopUser
		if q == "" {
			list, err = s.ListUsers(80)
		} else {
			list, err = s.FindUsers(q, 50)
		}
		if err == nil {
			jsonObj(c, list, nil)
			return
		}
	case "user":
		var id int64
		if id, err = formID(c, "tgId"); err == nil {
			u := s.User(id)
			if u == nil {
				err = common.NewError("user not found")
			} else {
				orders, _ := s.Orders("", id, 20)
				tickets, _ := s.Tickets(id, false, 10)
				jsonObj(c, map[string]any{
					"user":    u,
					"summary": s.Summary(id),
					"balance": s.Balance(id),
					"orders":  orders,
					"tickets": tickets,
				}, nil)
				return
			}
		}
	case "ticketMsgs":
		var id int64
		if id, err = formID(c, "id"); err == nil {
			var msgs []model.ShopTicketMsg
			msgs, err = s.TicketMsgs(uint(id), 80)
			if err == nil {
				jsonObj(c, msgs, nil)
				return
			}
		}
	case "ticketAnswer":
		var id int64
		if id, err = formID(c, "id"); err == nil {
			if service.ShopTicketNotify == nil {
				err = service.ErrBotDown
			} else {
				text := strings.TrimSpace(c.PostForm("text"))
				var tk *model.ShopTicket
				tk, err = s.TicketAnswer(uint(id), 0, text)
				if err == nil {
					service.ShopTicketNotify(tk, text, false)
				}
			}
		}
	case "ticketClose":
		var id int64
		if id, err = formID(c, "id"); err == nil {
			var tk *model.ShopTicket
			tk, err = s.TicketClose(uint(id), 0)
			if err == nil && service.ShopTicketNotify != nil {
				service.ShopTicketNotify(tk, "", true)
			}
		}
	case "broadcast":
		if service.ShopBroadcaster == nil {
			err = service.ErrBotDown
		} else {
			text := strings.TrimSpace(c.PostForm("text"))
			if text == "" || len([]rune(text)) > 4000 {
				err = common.NewError("broadcast text is empty or too long")
			} else {
				aud := strings.TrimSpace(c.PostForm("audience"))
				if aud == "all" {
					aud = ""
				}
				var ids []int64
				ids, err = s.AudienceTargets(aud, time.Now())
				if err == nil {
					sent, failed := service.ShopBroadcaster(ids, text)
					jsonMsgObj(c, "save", map[string]any{"sent": sent, "failed": failed, "total": len(ids)}, nil)
					return
				}
			}
		}
	default:
		err = common.NewError("unknown shop object")
	}
	jsonMsg(c, "save", err)
}
