package tgbot

import (
	"strings"
)

// The connection guide: which app to install on each platform, and how to
// add the subscription to it.

type guideApp struct {
	name  string
	links []string
}

type guidePlatform struct {
	key, fa, en string
	apps        []guideApp
}

var guidePlatforms = []guidePlatform{
	{"android", "🤖 اندروید", "🤖 Android", []guideApp{
		{"v2rayNG", []string{"https://play.google.com/store/apps/details?id=com.v2ray.ang", "https://github.com/2dust/v2rayNG/releases/latest"}},
		{"Hiddify", []string{"https://play.google.com/store/apps/details?id=app.hiddify.com", "https://github.com/hiddify/hiddify-app/releases/latest"}},
	}},
	{"ios", "🍏 آیفون و آیپد", "🍏 iPhone and iPad", []guideApp{
		{"Streisand", []string{"https://apps.apple.com/app/id6450534064"}},
		{"V2Box", []string{"https://apps.apple.com/app/id6446814690"}},
		{"v2RayTun", []string{"https://apps.apple.com/app/id6476628951"}},
		{"Shadowrocket", []string{"https://apps.apple.com/app/id932747118"}},
	}},
	{"windows", "🪟 ویندوز", "🪟 Windows", []guideApp{
		{"v2rayN", []string{"https://github.com/2dust/v2rayN/releases/latest"}},
		{"Hiddify", []string{"https://github.com/hiddify/hiddify-app/releases/latest"}},
	}},
	{"mac", "💻 مک", "💻 macOS", []guideApp{
		{"Hiddify", []string{"https://github.com/hiddify/hiddify-app/releases/latest"}},
		{"v2rayN", []string{"https://github.com/2dust/v2rayN/releases/latest"}},
	}},
	{"linux", "🐧 لینوکس", "🐧 Linux", []guideApp{
		{"Hiddify", []string{"https://github.com/hiddify/hiddify-app/releases/latest"}},
		{"v2rayN", []string{"https://github.com/2dust/v2rayN/releases/latest"}},
	}},
}

// guideScreen is the list of platforms, or the guide of one.
func (b *bot) guideScreen(platform string) (string, [][]button) {
	t := b.tr
	for _, p := range guidePlatforms {
		if p.key != platform {
			continue
		}
		lines := []string{"📚 <b>" + t("آموزش اتصال", "How to connect") + " · " + t(p.fa, p.en) + "</b>", "",
			t("۱. یکی از این برنامه‌ها را نصب کنید:", "1. Install one of these apps:")}
		for _, a := range p.apps {
			links := make([]string, 0, len(a.links))
			for i, l := range a.links {
				label := t("دانلود", "Download")
				if i > 0 {
					label = t("نسخهٔ دیگر", "Other build")
				}
				links = append(links, `<a href="`+esc(l)+`">`+label+`</a>`)
			}
			lines = append(lines, "• <b>"+a.name+"</b>: "+strings.Join(links, " · "))
		}
		lines = append(lines,
			t("۲. در «📦 سرویس‌های من» لینک اشتراک سرویس را کپی کنید (یا QR آن را اسکن کنید).",
				"2. Copy the subscription link of your service in «📦 My services» (or scan its QR code)."),
			t("۳. در برنامه دکمهٔ + را بزنید و «Import from clipboard» (افزودن از کلیپ‌بورد) را انتخاب کنید.",
				"3. In the app, tap + and pick «Import from clipboard»."),
			t("۴. اشتراک را به‌روز کنید، یک سرور انتخاب کنید و وصل شوید.",
				"4. Update the subscription, pick a server and connect."))
		if s := b.shopSvc().Settings().Support; strings.TrimSpace(s) != "" {
			lines = append(lines, "", "💬 "+t("مشکل داشتید؟ پشتیبانی: ", "Stuck? Support: ")+esc(s))
		}
		return strings.Join(lines, "\n"), [][]button{{{Text: t("🔙 سیستم‌عامل دیگر", "🔙 Another platform"), Data: "p:guide"}}, b.homeRow()}
	}
	var kb [][]button
	var row []button
	for _, p := range guidePlatforms {
		row = append(row, button{Text: t(p.fa, p.en), Data: "p:guide:" + p.key})
		if len(row) == 2 {
			kb = append(kb, row)
			row = nil
		}
	}
	if len(row) > 0 {
		kb = append(kb, row)
	}
	kb = append(kb, b.homeRow())
	return "📚 <b>" + t("آموزش اتصال", "How to connect") + "</b>\n" + t("سیستم‌عامل دستگاهتان را انتخاب کنید:", "Pick the platform of your device:"), kb
}

// supportText tells how to reach the support.
func (b *bot) supportText() string {
	t := b.tr
	if s := strings.TrimSpace(b.shopSvc().Settings().Support); s != "" {
		return "💬 <b>" + t("پشتیبانی", "Support") + "</b>\n" + t("برای پشتیبانی پیام بدهید به: ", "For help, message: ") + esc(s)
	}
	return "💬 " + t("راه ارتباط با پشتیبانی هنوز تنظیم نشده است.", "No support contact is set yet.")
}
