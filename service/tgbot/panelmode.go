package tgbot

import "strings"

// Private mode reserves global management for the owner and full partners.
// Sellers retain their explicitly permitted client tools and their own shop.
// Outside private mode the legacy panel-management switch applies.

// panelSections are the sections that are the panel's own management.
var panelSections = map[string]bool{
	"inbounds": true, "outbounds": true, "endpoints": true, "services": true, "tls": true,
	"routing": true, "settings": true, "logs": true,
}

// nodeEditVerbs change a node's definition rather than watch it.
var nodeEditVerbs = map[string]bool{"j": true, "e": true, "n": true, "nt": true, "d": true, "dy": true}

// panelOff tells that a section is hidden because the bot leaves the panel's
// management to the web panel.
func (b *bot) panelOff(section string) bool {
	if b.cfg.Private {
		if b.canManageAdmins() {
			return false
		}
		// Sellers keep only explicitly permitted client tools and their own shop.
		return section != "clients"
	}
	return !b.cfg.Panel && panelSections[section]
}

// keepAllowed drops the buttons the administrator being served cannot use,
// and the rows left empty.
func (b *bot) keepAllowed(kb [][]button) [][]button {
	out := make([][]button, 0, len(kb))
	for _, row := range kb {
		var kept []button
		for _, btn := range row {
			if btn.Data == "x:ls" && (b.cfg.Private && b.panelOff("stats") || !b.cfg.Private && !b.cfg.Panel) {
				continue
			}
			if btn.URL != "" || b.allowed(strings.Split(btn.Data, ":")) {
				kept = append(kept, btn)
			}
		}
		if len(kept) > 0 {
			out = append(out, kept)
		}
	}
	return out
}

// panelHelp leaves out of the help the commands and sections the bot does
// not offer while the panel's management is left to the web panel.
func (b *bot) panelHelp(help string) string {
	if b.cfg.Panel && !b.cfg.Private || b.cfg.Private && b.canManageAdmins() {
		return help
	}
	help = strings.NewReplacer(
		"(خانه، کلاینت‌ها، اینباندها، اوت‌باندها، اندپوینت‌ها، سرویس‌ها، TLS، نودها، قوانین، DNS، تنظیمات، آمار، لاگ، مدیران)", "(خانه، کلاینت‌ها، نودها، آمار، مدیران، فروشگاه)",
		"(home, clients, inbounds, outbounds, endpoints, services, TLS, nodes, rules, DNS, settings, stats, logs, admins)", "(home, clients, nodes, stats, admins, shop)",
	).Replace(help)
	lines := strings.Split(help, "\n")
	out := lines[:0]
	for _, l := range lines {
		cmd, _ := parseCommand(l)
		if sec, ok := commandSections[cmd]; ok && strings.HasPrefix(l, "/") && b.panelOff(sec) {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}
