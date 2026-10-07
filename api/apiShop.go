package api

import (
	"encoding/json"
	"strconv"
	"strings"

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
	jsonObj(c, map[string]any{
		"stats":     stats,
		"plans":     plans,
		"discounts": codes,
		"orders":    orders,
		"settings":  s.RawSettings(),
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
	default:
		err = common.NewError("unknown shop object")
	}
	jsonMsg(c, "save", err)
}
