package tgbot

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func TestShopDashboardCounts(t *testing.T) {
	e, _ := shopEnv(t)
	e.say(buyer, "/start")
	e.press(buyer, "p:trial")
	n := len(e.got())
	e.press(42, "q:menu")
	txt := lastText(e.since(n), "editMessageText")
	b := e.b
	if !strings.Contains(txt, fmt.Sprintf(b.tr("🟢 کاربران فعال امروز: %d", "🟢 Active today: %d"), 1)) {
		t.Fatalf("dashboard: %q", txt)
	}

	now := time.Unix(1_000_000, 0)
	day := int64(86400)
	clients := []model.Client{
		{Enable: true},                                        // active, unlimited
		{Enable: true, Expiry: now.Unix() + day},              // expiring
		{Enable: true, Expiry: now.Unix() + 10*day},           // active
		{Enable: true, Expiry: now.Unix() - 1},                // expired
		{Enable: true, Volume: 10, Up: 6, Down: 4},            // out of volume
		{Enable: false, Expiry: now.Unix() + 10*day},          // disabled
		{Enable: true, Volume: 10, Up: 1, Expiry: now.Unix()}, // expires now
	}
	if a, off, soon := serviceCounts(clients, now, 3); a != 3 || off != 4 || soon != 1 {
		t.Fatalf("counts %d active, %d off, %d expiring", a, off, soon)
	}
}
