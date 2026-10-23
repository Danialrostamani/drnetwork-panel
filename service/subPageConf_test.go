package service

import (
	"strings"
	"testing"
)

func TestSubPageConfIsChecked(t *testing.T) {
	out, err := normalizeSubPageConf(`{"name":"  Dr  ","support":"@help","channel":"https://t.me/news","accent":"#0c96b2","hideNet":true}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"name":"Dr"`, `"support":"https://t.me/help"`, `"hideNet":true`} {
		if !strings.Contains(out, want) {
			t.Fatalf("%s misses %s", out, want)
		}
	}
	if out, err := normalizeSubPageConf(""); err != nil || out != "{}" {
		t.Fatalf("empty: %q %v", out, err)
	}
	for _, bad := range []string{
		`{`,
		`{"support":"javascript:alert(1)"}`,
		`{"channel":"t.me/x"}`,
		`{"accent":"red"}`,
		`{"logo":"data:image/png;base64,AAAA"}`,
		`{"name":"` + strings.Repeat("x", 61) + `"}`,
	} {
		if _, err := normalizeSubPageConf(bad); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
	if PageLink("tg://resolve?domain=x") == "" || PageLink("ftp://x") != "" || PageLink("@a b") != "" {
		t.Fatal("PageLink")
	}
}
