package util

import (
	"strings"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func TestRenderNameDropsEmptyVariablesWithTheirSeparators(t *testing.T) {
	vars := map[string]string{"REMARK": "Ali", "INBOUND": "vless", "NODE": ""}
	cases := map[string]string{
		"{REMARK} | {NODE} | {INBOUND}": "Ali | vless",
		"{NODE} - {REMARK}":             "Ali",
		"{REMARK} - {NODE}":             "Ali",
		"🇩🇪 {REMARK} {UNKNOWN}":         "🇩🇪 Ali {UNKNOWN}",
		"{NODE}":                        "",
		"{NODE} | {GROUP}":              "",
		"plain":                         "plain",
	}
	for tpl, want := range cases {
		if got := RenderName(tpl, vars); got != want {
			t.Errorf("RenderName(%q) = %q, want %q", tpl, got, want)
		}
	}
	long := RenderName("{REMARK}", map[string]string{"REMARK": strings.Repeat("ب", 300)})
	if n := len([]rune(long)); n != MaxLinkName {
		t.Fatalf("a long name has %d characters, want %d", n, MaxLinkName)
	}
}

func TestCheckNameTemplate(t *testing.T) {
	for _, ok := range []string{"", "{USER}", "x {REMAINING} / {DAYS_LEFT}d", "{EXPIRE_JALALI}"} {
		if err := CheckNameTemplate(ok); err != nil {
			t.Errorf("CheckNameTemplate(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"no variable", "{user}", "{USER}\n", strings.Repeat("x", 250) + "{USER}{USER}"} {
		if err := CheckNameTemplate(bad); err == nil {
			t.Errorf("CheckNameTemplate(%q) passed", bad)
		}
	}
}

func TestGregorianToJalali(t *testing.T) {
	cases := []struct{ gy, gm, gd, jy, jm, jd int }{
		{2024, 3, 20, 1403, 1, 1},
		{2023, 3, 21, 1402, 1, 1},
		{2024, 12, 31, 1403, 10, 11},
		{2025, 3, 20, 1403, 12, 30},
		{2025, 3, 21, 1404, 1, 1},
		{2000, 1, 1, 1378, 10, 11},
	}
	for _, c := range cases {
		jy, jm, jd := GregorianToJalali(c.gy, c.gm, c.gd)
		if jy != c.jy || jm != c.jm || jd != c.jd {
			t.Errorf("%d-%d-%d = %d/%d/%d, want %d/%d/%d", c.gy, c.gm, c.gd, jy, jm, jd, c.jy, c.jm, c.jd)
		}
	}
}

func TestClientNameVars(t *testing.T) {
	loc := time.UTC
	now := time.Date(2024, 3, 10, 12, 0, 0, 0, loc)
	c := &model.Client{Name: "u1", Remark: "Ali", Volume: 10 << 30, Up: 1 << 30, Down: 1 << 30,
		Expiry: time.Date(2024, 3, 20, 12, 0, 0, 0, loc).Unix()}
	v := ClientNameVars(c, now, loc)
	want := map[string]string{"USER": "u1", "REMARK": "Ali", "TOTAL": "10GB", "USED": "2GB", "REMAINING": "8GB",
		"DAYS_LEFT": "10", "EXPIRE": "2024-03-20", "EXPIRE_JALALI": "1403/01/01"}
	for k, w := range want {
		if v[k] != w {
			t.Errorf("%s = %q, want %q", k, v[k], w)
		}
	}
	if v := ClientNameVars(&model.Client{Name: "u2"}, now, loc); v["TOTAL"] != "∞" || v["DAYS_LEFT"] != "∞" || v["EXPIRE"] != "" {
		t.Errorf("unlimited client: %v", v)
	}
}
