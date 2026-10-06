package tgbot

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"

	qrcode "github.com/skip2/go-qrcode"
)

const gib = int64(1) << 30

var clientNameRe = regexp.MustCompile(`^[A-Za-z0-9_.@-]{1,64}$`)

const randomAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func randomSeq(n int) string {
	out := make([]byte, n)
	max := big.NewInt(int64(len(randomAlphabet)))
	for i := range out {
		v, err := rand.Int(rand.Reader, max)
		if err != nil {
			panic(err)
		}
		out[i] = randomAlphabet[v.Int64()]
	}
	return string(out)
}

func randomBase64(n int) string {
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func randomUUID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16])
}

// newClientConfig mirrors randomConfigs() in the web panel: one credential set
// per protocol, so the client works on every inbound type.
func newClientConfig(name string) json.RawMessage {
	pass := randomSeq(10)
	ss16, ss32 := randomBase64(16), randomBase64(32)
	id := randomUUID()
	m := map[string]map[string]interface{}{
		"mixed":         {"username": name, "password": pass},
		"socks":         {"username": name, "password": pass},
		"http":          {"username": name, "password": pass},
		"shadowsocks":   {"name": name, "password": ss32},
		"shadowsocks16": {"name": name, "password": ss16},
		"shadowtls":     {"name": name, "password": ss32},
		"vmess":         {"name": name, "uuid": id, "alterId": 0},
		"vless":         {"name": name, "uuid": id, "flow": "xtls-rprx-vision"},
		"anytls":        {"name": name, "password": pass},
		"trojan":        {"name": name, "password": pass},
		"naive":         {"username": name, "password": pass},
		"hysteria":      {"name": name, "auth_str": pass},
		"snell":         {"name": name, "userkey": randomSeq(32)},
		"tuic":          {"name": name, "uuid": id, "password": pass},
		"hysteria2":     {"name": name, "password": pass},
	}
	out, _ := json.Marshal(m)
	return out
}

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
	// An administrator with a volume limit pays for the volume this write
	// hands out before it is saved, and gets it back if the save fails.
	charge, err := b.chargeVolume(act, payload)
	if err != nil {
		return err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		charge.refund()
		return err
	}
	if _, err := b.configService.Save("clients", act, data, "", b.actor(), b.host()); err != nil {
		charge.refund()
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
