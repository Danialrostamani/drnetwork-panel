package tgbot

import (
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/service"
)

func TestWelcomeHTMLKeepsSafeTags(t *testing.T) {
	got := welcomeHTML(`hello <b>gold</b> and [ch](https://t.me/x) <script>x</script>`)
	if !strings.Contains(got, "<b>gold</b>") {
		t.Fatalf("bold: %s", got)
	}
	if !strings.Contains(got, `<a href="https://t.me/x">ch</a>`) {
		t.Fatalf("link: %s", got)
	}
	if strings.Contains(got, "<script>") {
		t.Fatalf("script survived: %s", got)
	}
}

func TestShopHomeShowsWelcome(t *testing.T) {
	e, _ := shopEnv(t)
	s := &service.ShopService{}
	if err := s.SetSetting("shopWelcome", "🎉 <b>تعرفه</b>"); err != nil {
		t.Fatal(err)
	}
	text, _ := e.b.shopHome(buyer)
	if !strings.Contains(text, "<b>تعرفه</b>") || !strings.Contains(text, "Shop") {
		t.Fatalf("home: %s", text)
	}
}
