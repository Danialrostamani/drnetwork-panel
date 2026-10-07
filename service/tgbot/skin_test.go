package tgbot

import (
	"strings"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func TestColorBar(t *testing.T) {
	if got := colorBar(30, 10); got != strings.Repeat("🟩", 3)+strings.Repeat("⬜", 7) {
		t.Fatalf("30%%: %q", got)
	}
	if got := colorBar(70, 10); !strings.HasPrefix(got, "🟨") {
		t.Fatalf("70%% should be yellow: %q", got)
	}
	if got := colorBar(150, 4); got != strings.Repeat("🟥", 4) {
		t.Fatalf("over 100%%: %q", got)
	}
	if got := colorBar(-5, 4); got != strings.Repeat("⬜", 4) {
		t.Fatalf("below 0: %q", got)
	}
}

func TestSkinChoice(t *testing.T) {
	if normSkin("Classic ") != skinClassic || normSkin("") != skinColorful || normSkin("x") != skinColorful {
		t.Fatal("normSkin")
	}
	b := &bot{}
	if !b.colorful() || b.bar(50, 2) != "🟩⬜" {
		t.Fatal("colorful is the default look")
	}
	b.cfg.Skin = skinClassic
	if b.colorful() || b.bar(50, 2) != "▰▱" {
		t.Fatal("classic look")
	}
}

func TestRegrid(t *testing.T) {
	kb := [][]button{{{Data: "a"}, {Data: "b"}}, {{Data: "c"}, {Data: "d"}}, {{Data: "e"}}}
	out := regrid(kb, 3)
	if len(out) != 2 || len(out[0]) != 3 || len(out[1]) != 2 || out[1][1].Data != "e" {
		t.Fatalf("regrid: %+v", out)
	}
}

func TestExpiryBar(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	c := model.Client{CreatedAt: now.Unix() - 25*86400, Expiry: now.Unix() + 75*86400}
	b := &bot{}
	if got := b.expiryBar(c, now); !strings.Contains(got, "25%") {
		t.Fatalf("expiry bar: %q", got)
	}
	c.Expiry = 0
	if b.expiryBar(c, now) != "" {
		t.Fatal("no bar without an end date")
	}
	b.cfg.Skin = skinClassic
	c.Expiry = now.Unix() + 86400
	if b.expiryBar(c, now) != "" {
		t.Fatal("no bar in the classic look")
	}
}
