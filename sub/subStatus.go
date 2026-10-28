package sub

import (
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"

	"github.com/gin-gonic/gin"
)

// The status page asks the link itself (?format=status) for what the
// subscription headers cannot carry: whether the client is connected now,
// when it was last, what it used in the last day and week, and on which
// server. Reading the nodes' history is not free, so an answer is kept a
// little while for each client.

const statusTTL = 45 * time.Second

type pageStatus struct {
	OK     bool  `json:"ok"`
	Now    int64 `json:"now"`
	Client struct {
		Online    bool  `json:"online"`
		LastSeen  int64 `json:"lastSeen,omitempty"`
		CreatedAt int64 `json:"createdAt,omitempty"`
	} `json:"client"`
	Usage    map[string]int64         `json:"usage,omitempty"`
	Inbounds []service.ClientLocation `json:"inbounds,omitempty"`
	Servers  []service.ServerHealth   `json:"servers,omitempty"`
	Devices  *pageDevices             `json:"devices,omitempty"`
	Days     []service.DayUsage       `json:"days,omitempty"`
	Daily    *pageDaily               `json:"daily,omitempty"`
}

// pageDaily is the client's allowance for one day and what of it went today.
type pageDaily struct {
	Volume int64 `json:"volume,omitempty"`
	Time   int64 `json:"time,omitempty"`
	Used   int64 `json:"used"`
	Online int64 `json:"online"`
	Locked bool  `json:"locked"`
}

type pageDevices struct {
	Used  int `json:"used"`
	Limit int `json:"limit"`
}

type statusEntry struct {
	at  time.Time
	out pageStatus
}

var (
	statusMu    sync.Mutex
	statusCache = map[string]statusEntry{}
)

// statusCacheMax bounds the cache; past it the stale entries go.
const statusCacheMax = 5000

func (s *SubHandler) status(c *gin.Context, subID string) {
	if s.SettingService.GetSubPageConf().HideLive {
		c.String(404, "")
		return
	}
	var client model.Client
	if err := database.GetDB().Where("name = ?", subID).First(&client).Error; err != nil {
		c.String(400, "Error!")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, cachedStatus(client, time.Now()))
}

func cachedStatus(client model.Client, now time.Time) pageStatus {
	statusMu.Lock()
	e, ok := statusCache[client.Name]
	statusMu.Unlock()
	if ok && now.Sub(e.at) < statusTTL {
		return e.out
	}
	out := buildStatus(client, now)
	statusMu.Lock()
	if len(statusCache) >= statusCacheMax {
		for k, v := range statusCache {
			if now.Sub(v.at) >= statusTTL {
				delete(statusCache, k)
			}
		}
		if len(statusCache) >= statusCacheMax {
			statusCache = map[string]statusEntry{}
		}
	}
	statusCache[client.Name] = statusEntry{at: now, out: out}
	statusMu.Unlock()
	return out
}

func buildStatus(client model.Client, now time.Time) pageStatus {
	st := &service.StatsService{}
	var out pageStatus
	out.OK = true
	out.Now = now.Unix()
	out.Client.Online = st.IsClientOnline(client.Name)
	out.Client.LastSeen = client.OnlineAt
	if out.Client.Online {
		out.Client.LastSeen = now.Unix()
	}
	out.Client.CreatedAt = client.CreatedAt
	if day, week, ok := st.ClientRecentUsage(client, now.Unix()); ok {
		out.Usage = map[string]int64{"h24": day, "d7": week}
	}
	out.Inbounds = st.ClientLocations(client)
	if !service.SubPageFlags().HideServers {
		out.Servers = st.ClientServers(out.Inbounds)
	}
	if used, limit := service.ClientDevices(client); limit > 0 {
		out.Devices = &pageDevices{Used: used, Limit: limit}
	}
	days := service.ClientDays(client.Id, 30, now)
	for _, d := range days {
		if d.Up+d.Down > 0 {
			out.Days = days
			break
		}
	}
	if client.DailyVolume > 0 || client.DailyTime > 0 {
		d := &pageDaily{Volume: client.DailyVolume, Time: client.DailyTime, Locked: client.DayLocked}
		if client.DayKey == service.DayKey(now) {
			d.Used, d.Online = client.DayUsed, client.DayOnline
		}
		out.Daily = d
	}
	return out
}

// vpnSide is what the page learns about one address its user comes from.
type vpnSide struct {
	IP   string `json:"ip,omitempty"`
	Ours bool   `json:"ours"`
	Via  string `json:"via,omitempty"`
}

// vpn tells the page whether its user's traffic comes out of one of the
// servers: the address this request came from, and the one an outside
// service saw (ip=), which the page asks for itself.
func (s *SubHandler) vpn(c *gin.Context, subID string) {
	if s.SettingService.GetSubPageConf().HideVpn {
		c.String(404, "")
		return
	}
	var client model.Client
	if err := database.GetDB().Where("name = ?", subID).First(&client).Error; err != nil {
		c.String(400, "Error!")
		return
	}
	locs := (&service.StatsService{}).ClientLocations(client)
	side := func(ip string) vpnSide {
		out := vpnSide{IP: ip}
		if node, ok := service.ServerOf(ip); ok {
			out.Ours = true
			for _, l := range locs {
				if l.Node == node {
					out.Via = l.Name
					break
				}
			}
		}
		return out
	}
	res := map[string]vpnSide{"self": side(subRequestIP(c))}
	if ext, ok := c.GetQuery("ip"); ok {
		if a, err := netip.ParseAddr(strings.TrimSpace(ext)); err == nil {
			res["ext"] = side(a.String())
		}
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, res)
}
