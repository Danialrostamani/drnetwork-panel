package tgbot

import "fmt"

var messages = map[string]map[string]string{
	"fa": {
		"help": "🤖 <b>ربات مدیریت DrNetwork</b>\n\n" +
			"/status — وضعیت پنل و کل کلاستر\n" +
			"/nodes — وضعیت نودها\n" +
			"/online — کاربران آنلاین\n" +
			"/clients — کلاینت‌های نزدیک به اتمام حجم یا زمان\n" +
			"/clients <code>نام</code> — جستجو یا جزئیات یک کلاینت\n" +
			"/ips <code>نام</code> — IP های آنلاین یک کلاینت\n" +
			"/inbounds — اینباندها و وضعیت آنلاین\n" +
			"/traffic — مصرف کل و پرمصرف‌ترین‌ها\n" +
			"/id — شناسه تلگرام شما",
		"yourId":        "شناسه تلگرام شما: <code>%d</code>\nاین عدد را در تنظیمات پنل، بخش ربات تلگرام، به‌عنوان ادمین وارد کنید.",
		"denied":        "شما دسترسی ندارید. با /id شناسه خود را بگیرید و به مدیر پنل بدهید.",
		"unknown":       "دستور ناشناخته است. /help",
		"status":        "📊 <b>DrNetwork %s</b>",
		"cpuMem":        "پردازنده: %.0f٪ · حافظه: %s / %s",
		"core":          "هسته: %s · مدت فعالیت: %s",
		"running":       "فعال",
		"stopped":       "متوقف",
		"nodesLine":     "نودها: %d از %d آنلاین",
		"clientsLine":   "کلاینت‌ها: %d (فعال: %d) · آنلاین الان: %d",
		"trafficLine":   "مصرف کل: ↑ %s ↓ %s",
		"noNodes":       "هیچ نودی تعریف نشده است.",
		"nodeOnline":    "آنلاین",
		"nodeOffline":   "آفلاین",
		"nodeCore":      "هسته متوقف",
		"lastSeen":      "آخرین حضور",
		"never":         "هرگز",
		"noOnline":      "الان هیچ کاربری آنلاین نیست.",
		"onlineTitle":   "🟢 <b>کاربران آنلاین (%d)</b>",
		"andMore":       "… و %d مورد دیگر",
		"noClients":     "کلاینتی پیدا نشد.",
		"nearTitle":     "⚠️ <b>کلاینت‌های نزدیک به اتمام</b>",
		"noNear":        "هیچ کلاینتی نزدیک به اتمام حجم یا زمان نیست.",
		"foundTitle":    "🔎 <b>نتیجه جستجو (%d)</b>",
		"unlimited":     "نامحدود",
		"expired":       "منقضی",
		"daysLeft":      "%d روز مانده",
		"hoursLeft":     "%d ساعت مانده",
		"delayStart":    "شروع از اولین اتصال (%d روز)",
		"usage":         "مصرف",
		"expiry":        "انقضا",
		"enabled":       "فعال",
		"disabled":      "غیرفعال",
		"group":         "گروه",
		"ipLimit":       "محدودیت IP",
		"lastOnline":    "آخرین اتصال",
		"createdAt":     "ساخته‌شده",
		"ipsUsage":      "نام کلاینت را بنویسید: /ips <code>نام</code>",
		"ipsTitle":      "🌐 <b>IP های آنلاین %s (%d)</b>",
		"noIps":         "الان IP ای ثبت نشده است.",
		"ipsNoLimit":    "ℹ️ IP فقط برای کلاینت‌هایی که محدودیت IP دارند ثبت می‌شود.",
		"inboundsHead":  "📡 <b>اینباندها (%d)</b>",
		"noInbounds":    "اینبانتی تعریف نشده است.",
		"trafficHead":   "📈 <b>مصرف کل</b>",
		"topUsers":      "پرمصرف‌ترین‌ها:",
		"nodeDown":      "🔴 <b>نود %s قطع شد</b>\n%s",
		"nodeUp":        "🟢 <b>نود %s دوباره آنلاین شد</b>\nمدت قطعی: %s",
		"alertTitle":    "⚠️ <b>هشدار کلاینت‌ها</b>",
		"alertVolume":   "نزدیک به اتمام حجم (%d٪)",
		"alertExpiry":   "کمتر از %d روز تا انقضا",
		"alertDepleted": "حجم تمام شده",
		"alertExpired":  "منقضی شده",
		"started":       "✅ ربات DrNetwork فعال شد.",
		"min":           "دقیقه",
		"hour":          "ساعت",
		"day":           "روز",
		"sec":           "ثانیه",
	},
	"en": {
		"help": "🤖 <b>DrNetwork management bot</b>\n\n" +
			"/status — panel and cluster status\n" +
			"/nodes — node status\n" +
			"/online — online users\n" +
			"/clients — clients close to their volume or time limit\n" +
			"/clients <code>name</code> — search or show one client\n" +
			"/ips <code>name</code> — online IPs of a client\n" +
			"/inbounds — inbounds and their online state\n" +
			"/traffic — total usage and top users\n" +
			"/id — your Telegram ID",
		"yourId":        "Your Telegram ID: <code>%d</code>\nAdd it as an admin in the panel settings, Telegram bot section.",
		"denied":        "You are not allowed to use this bot. Send /id and give the number to the panel admin.",
		"unknown":       "Unknown command. /help",
		"status":        "📊 <b>DrNetwork %s</b>",
		"cpuMem":        "CPU: %.0f%% · Memory: %s / %s",
		"core":          "Core: %s · Uptime: %s",
		"running":       "running",
		"stopped":       "stopped",
		"nodesLine":     "Nodes: %d of %d online",
		"clientsLine":   "Clients: %d (enabled: %d) · online now: %d",
		"trafficLine":   "Total usage: ↑ %s ↓ %s",
		"noNodes":       "No nodes are configured.",
		"nodeOnline":    "online",
		"nodeOffline":   "offline",
		"nodeCore":      "core stopped",
		"lastSeen":      "Last seen",
		"never":         "never",
		"noOnline":      "Nobody is online right now.",
		"onlineTitle":   "🟢 <b>Online users (%d)</b>",
		"andMore":       "… and %d more",
		"noClients":     "No clients found.",
		"nearTitle":     "⚠️ <b>Clients close to their limit</b>",
		"noNear":        "No client is close to its volume or time limit.",
		"foundTitle":    "🔎 <b>Search results (%d)</b>",
		"unlimited":     "unlimited",
		"expired":       "expired",
		"daysLeft":      "%d days left",
		"hoursLeft":     "%d hours left",
		"delayStart":    "starts on first use (%d days)",
		"usage":         "Usage",
		"expiry":        "Expiry",
		"enabled":       "enabled",
		"disabled":      "disabled",
		"group":         "Group",
		"ipLimit":       "IP limit",
		"lastOnline":    "Last online",
		"createdAt":     "Created",
		"ipsUsage":      "Give a client name: /ips <code>name</code>",
		"ipsTitle":      "🌐 <b>Online IPs of %s (%d)</b>",
		"noIps":         "No IP is recorded right now.",
		"ipsNoLimit":    "ℹ️ IPs are only recorded for clients that have an IP limit.",
		"inboundsHead":  "📡 <b>Inbounds (%d)</b>",
		"noInbounds":    "No inbounds are configured.",
		"trafficHead":   "📈 <b>Total usage</b>",
		"topUsers":      "Top users:",
		"nodeDown":      "🔴 <b>Node %s is down</b>\n%s",
		"nodeUp":        "🟢 <b>Node %s is back online</b>\nDowntime: %s",
		"alertTitle":    "⚠️ <b>Client alerts</b>",
		"alertVolume":   "close to its volume limit (%d%%)",
		"alertExpiry":   "less than %d days until expiry",
		"alertDepleted": "volume used up",
		"alertExpired":  "expired",
		"started":       "✅ DrNetwork bot is running.",
		"min":           "min",
		"hour":          "h",
		"day":           "d",
		"sec":           "s",
	},
}

func (b *bot) t(key string, args ...interface{}) string {
	text, ok := messages[b.cfg.Lang][key]
	if !ok {
		text = messages["en"][key]
	}
	if len(args) == 0 {
		return text
	}
	return fmt.Sprintf(text, args...)
}
