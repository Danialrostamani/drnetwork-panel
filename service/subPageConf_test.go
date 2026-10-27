package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
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

// Notices keep their kind and end date and go away after it; services to
// check need an https address.
func TestSubPageNoticesAndSites(t *testing.T) {
	out, err := normalizeSubPageConf(`{"notice":"old","notices":[{"text":" a ","until":"2030-01-02"},{"text":"","textEn":""},{"text":"b","kind":"promo"}],"sites":[{"name":"G","url":"https://g.co/x"},{"name":"","url":""}]}`)
	if err != nil {
		t.Fatal(err)
	}
	var conf SubPageConf
	if err := json.Unmarshal([]byte(out), &conf); err != nil {
		t.Fatal(err)
	}
	if len(conf.Notices) != 2 || conf.Notices[0].Text != "a" || conf.Notices[0].Kind != "info" || conf.Notices[1].Kind != "promo" || len(conf.Sites) != 1 {
		t.Fatalf("normalized %s", out)
	}
	loc := time.FixedZone("x", 3*3600)
	before := conf.ActiveNotices(time.Date(2030, 1, 2, 23, 0, 0, 0, loc), loc)
	after := conf.ActiveNotices(time.Date(2030, 1, 3, 0, 1, 0, 0, loc), loc)
	if len(before) != 3 || before[0].Text != "old" || len(after) != 2 || after[1].Text != "b" {
		t.Fatalf("before %v after %v", before, after)
	}
	for _, bad := range []string{
		`{"notices":[{"text":"x","kind":"red"}]}`,
		`{"notices":[{"text":"x","until":"tomorrow"}]}`,
		`{"sites":[{"name":"x","url":"http://g.co"}]}`,
		`{"sites":[{"name":"","url":"https://g.co"}]}`,
		`{"notices":[` + strings.Repeat(`{"text":"x"},`, 10) + `{"text":"x"}]}`,
	} {
		if _, err := normalizeSubPageConf(bad); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}
