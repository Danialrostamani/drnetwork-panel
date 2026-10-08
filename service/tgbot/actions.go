package tgbot

import (
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
	"github.com/Danialrostamani/drnetwork-panel/util"

	qrcode "github.com/skip2/go-qrcode"
)

const gib = int64(1) << 30

var clientNameRe = regexp.MustCompile(`^[A-Za-z0-9_.@-]{1,64}$`)

// The credential helpers live in util, shared with the panel's own saves.
var (
	randomSeq       = util.RandomSeq
	randomBase64    = util.RandomBase64
	randomUUID      = util.RandomUUID
	newClientConfig = util.NewClientConfig
)

// host is the address written into generated client links and the
// subscription URL: the sub or web domain if one is set, else the machine's
// first public-looking IPv4 address.
func (b *bot) host() string {
	ss := &service.SettingService{}
	if d, _ := ss.GetSubDomain(); strings.TrimSpace(d) != "" {
		return strings.TrimSpace(d)
	}
	if d, _ := ss.GetWebDomain(); strings.TrimSpace(d) != "" {
		return strings.TrimSpace(d)
	}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		var private string
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil || ipNet.IP.IsLoopback() || ipNet.IP.IsLinkLocalUnicast() {
				continue
			}
			if ipNet.IP.IsPrivate() {
				if private == "" {
					private = ipNet.IP.String()
				}
				continue
			}
			return ipNet.IP.String()
		}
		if private != "" {
			return private
		}
	}
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return "localhost"
}

func (b *bot) subLink(name string) (string, error) {
	base, err := (&service.SettingService{}).GetFinalSubURI(b.host())
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	return base + url.PathEscape(name), nil
}

func qrPNG(text string) ([]byte, error) {
	return qrcode.Encode(text, qrcode.Medium, 512)
}

func (b *bot) fanOut() {
	ns := &service.NodeSyncService{}
	ns.MarkAllDirty()
	go ns.ReconcileDirtyOnline()
}

func (b *bot) save(act string, payload interface{}) error {
	if b.configService == nil {
		return errors.New("config service unavailable")
	}
	if err := b.guardSave(act, payload); err != nil {
		return err
	}
	// An administrator whose volume limit is used up cannot give out more.
	if err := b.quotaGate(act, payload); err != nil {
		return err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// ConfigService counts the clients this creates against the limit, in the
	// same transaction as the creation.
	if _, err := b.configService.Save("clients", act, data, "", b.actor(), b.host()); err != nil {
		return err
	}
	b.fanOut()
	return nil
}

// edit loads the whole client row, applies fn and saves it through the same
// path as the web panel, so inbound users, links and the nodes follow.
func (b *bot) editClient(id uint, fn func(c *model.Client) error) error {
	c, err := b.fullClient(id)
	if err != nil {
		return err
	}
	if err := fn(c); err != nil {
		return err
	}
	return b.save("edit", c)
}

func (b *bot) setEnabled(id uint, enable bool) error {
	return b.editClient(id, func(c *model.Client) error { c.Enable = enable; return nil })
}

// resetTraffic zeroes up/down; ClientService folds the old values into the
// lifetime totals when it sees an edit with both at zero.
func (b *bot) resetTraffic(id uint) error {
	return b.editClient(id, func(c *model.Client) error { c.Up, c.Down = 0, 0; return nil })
}

func (b *bot) setVolume(id uint, bytes int64) error {
	return b.editClient(id, func(c *model.Client) error { c.Volume = bytes; return nil })
}

func (b *bot) addVolume(id uint, bytes int64) error {
	return b.editClient(id, func(c *model.Client) error {
		if c.Volume <= 0 {
			return errors.New("unlimited")
		}
		c.Volume += bytes
		return nil
	})
}

func (b *bot) setExpiryDays(id uint, days int) error {
	return b.editClient(id, func(c *model.Client) error {
		if c.DelayStart {
			return errors.New("delay start")
		}
		if days <= 0 {
			c.Expiry = 0
		} else {
			c.Expiry = time.Now().Add(time.Duration(days) * 24 * time.Hour).Unix()
		}
		return nil
	})
}

func (b *bot) addDays(id uint, days int) error {
	return b.editClient(id, func(c *model.Client) error {
		if c.DelayStart {
			return errors.New("delay start")
		}
		if c.Expiry <= 0 {
			return errors.New("unlimited")
		}
		base := time.Unix(c.Expiry, 0)
		if now := time.Now(); base.Before(now) {
			base = now
		}
		c.Expiry = base.Add(time.Duration(days) * 24 * time.Hour).Unix()
		return nil
	})
}

func (b *bot) setLimitIP(id uint, n int) error {
	return b.editClient(id, func(c *model.Client) error { c.LimitIp = n; return nil })
}

func (b *bot) deleteClient(id uint) error {
	return b.save("del", id)
}

func (b *bot) bindClient(id uint, tgID int64) error {
	if b.scope != "" && !b.storedInScope(id) {
		return b.outOfScope()
	}
	return database.GetDB().Model(&model.Client{}).Where("id = ?", id).Update("tg_id", tgID).Error
}

// createClient adds a client on every inbound, as the web panel's "new
// client" form does by default.
func (b *bot) createClient(name, group string, volume int64, days int, limitIP int) error {
	if !clientNameRe.MatchString(name) {
		return errors.New("bad name")
	}
	group, err := b.groupForCreate(group)
	if err != nil {
		return err
	}
	var ids []uint
	if err := database.GetDB().Model(&model.Inbound{}).Order("id").Pluck("id", &ids).Error; err != nil {
		return err
	}
	if ids == nil {
		ids = []uint{}
	}
	inbounds, _ := json.Marshal(ids)
	c := model.Client{
		Enable:   true,
		Name:     name,
		Config:   newClientConfig(name),
		Inbounds: inbounds,
		Links:    json.RawMessage(`[]`),
		Volume:   volume,
		LimitIp:  limitIP,
		Group:    group,
	}
	if days > 0 {
		c.Expiry = time.Now().Add(time.Duration(days) * 24 * time.Hour).Unix()
	}
	return b.save("new", c)
}
