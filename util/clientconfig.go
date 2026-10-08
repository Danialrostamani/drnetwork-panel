package util

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
)

const randomAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// RandomSeq returns n random letters and digits.
func RandomSeq(n int) string {
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

// RandomBase64 returns n random bytes, base64-encoded.
func RandomBase64(n int) string {
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// RandomUUID returns a random (version 4) UUID.
func RandomUUID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16])
}

// NewClientConfig mirrors randomConfigs() in the web panel: one credential set
// per protocol, so the client works on every inbound type.
func NewClientConfig(name string) json.RawMessage {
	pass := RandomSeq(10)
	ss16, ss32 := RandomBase64(16), RandomBase64(32)
	id := RandomUUID()
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
		"snell":         {"name": name, "userkey": RandomSeq(32)},
		"tuic":          {"name": name, "uuid": id, "password": pass},
		"hysteria2":     {"name": name, "password": pass},
	}
	out, _ := json.Marshal(m)
	return out
}
