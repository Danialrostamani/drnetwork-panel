package tgbot

// useAccess gives the bot the administrators a configuration describes, as the
// supervisor does when the settings change.
func useAccess(b *bot, cfg botConfig) {
	b.cfg.Owner, b.cfg.Admins, b.cfg.Scopes, b.cfg.Perms, b.cfg.Locked = cfg.Owner, cfg.Admins, cfg.Scopes, cfg.Perms, cfg.Locked
	b.setAccess(b.cfg.access())
}
