// Package tgbot is the panel's Telegram bot: read-only management commands for
// the configured administrators plus node and client alerts. It talks to the
// Bot API over plain HTTPS long polling, optionally through a proxy, so it
// needs no extra dependency and no inbound port.
package tgbot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

// apiBase is a variable so tests can point the bot at a fake server.
var apiBase = "https://api.telegram.org"

const (
	pollTimeoutSeconds = 25
	reloadInterval     = 15 * time.Second
	maxMessageRunes    = 3800
)

var (
	managerMu     sync.Mutex
	managerCancel context.CancelFunc
	managerDone   chan struct{}
)

// Start launches the supervisor. It re-reads the settings every few seconds, so
// enabling, disabling or re-keying the bot in the panel takes effect without a
// restart.
func Start(configService *service.ConfigService) {
	managerMu.Lock()
	defer managerMu.Unlock()
	if managerCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	managerCancel, managerDone = cancel, done
	go func() {
		defer close(done)
		supervise(ctx, configService)
	}()
}

// Stop ends the supervisor and the running bot, if any.
func Stop() {
	managerMu.Lock()
	cancel, done := managerCancel, managerDone
	managerCancel, managerDone = nil, nil
	managerMu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}

func supervise(ctx context.Context, configService *service.ConfigService) {
	var (
		current string
		stop    context.CancelFunc
		done    chan struct{}
	)
	halt := func() {
		if stop != nil {
			stop()
			<-done
			stop, done = nil, nil
		}
	}
	defer halt()
	ticker := time.NewTicker(reloadInterval)
	defer ticker.Stop()
	for {
		cfg, err := loadConfig()
		if err != nil {
			logger.Warning("telegram bot: read settings: ", err)
		} else if key := cfg.fingerprint(); key != current {
			halt()
			current = key
			if cfg.Enable && cfg.Token != "" {
				botCtx, cancel := context.WithCancel(ctx)
				finished := make(chan struct{})
				stop, done = cancel, finished
				b := newBot(cfg)
				b.configService = configService
				go func() {
					defer close(finished)
					b.run(botCtx)
				}()
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

type botConfig struct {
	Enable bool
	Token  string
	Admins []int64
	Proxy  string
	Lang   string
	Notify bool

	Report       string
	ReportBackup bool
}

func (c botConfig) fingerprint() string {
	return fmt.Sprint(c.Enable, "|", c.Token, "|", c.Admins, "|", c.Proxy, "|", c.Lang, "|", c.Notify, "|", c.Report, "|", c.ReportBackup)
}

func (c botConfig) isAdmin(id int64) bool {
	for _, a := range c.Admins {
		if a == id {
			return true
		}
	}
	return false
}

func parseAdmins(raw string) []int64 {
	var out []int64
	seen := map[int64]bool{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == ';' || r == '\n' }) {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func loadConfig() (botConfig, error) {
	s, err := (&service.SettingService{}).GetTgBotSettings()
	if err != nil {
		return botConfig{}, err
	}
	lang := strings.ToLower(strings.TrimSpace(s.Lang))
	if lang != "en" {
		lang = "fa"
	}
	return botConfig{
		Enable: s.Enable,
		Token:  strings.TrimSpace(s.Token),
		Admins: parseAdmins(s.Admins),
		Proxy:  strings.TrimSpace(s.Proxy),
		Lang:   lang,
		Notify: s.Notify,

		Report:       strings.TrimSpace(s.Report),
		ReportBackup: s.ReportBackup,
	}, nil
}

type bot struct {
	cfg           botConfig
	client        *http.Client
	loc           *time.Location
	configService *service.ConfigService
	pend          pendingStore
}

func newBot(cfg botConfig) *bot {
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment, IdleConnTimeout: 60 * time.Second}
	if cfg.Proxy != "" {
		if u, err := url.Parse(cfg.Proxy); err == nil && u.Host != "" {
			transport.Proxy = http.ProxyURL(u)
		} else {
			logger.Warning("telegram bot: ignoring invalid proxy URL")
		}
	}
	loc := time.Local
	if l, err := (&service.SettingService{}).GetTimeLocation(); err == nil && l != nil {
		loc = l
	}
	return &bot{
		cfg:    cfg,
		client: &http.Client{Timeout: (pollTimeoutSeconds + 20) * time.Second, Transport: transport},
		loc:    loc,
	}
}

// scrub keeps the bot token out of anything that reaches the log: net/http
// errors quote the full request URL, which carries it.
func (b *bot) scrub(s string) string {
	if b.cfg.Token == "" {
		return s
	}
	return strings.ReplaceAll(s, b.cfg.Token, "***")
}

func (b *bot) call(ctx context.Context, method string, params, out interface{}) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/bot"+b.cfg.Token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return errors.New(b.scrub(err.Error()))
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return errors.New(b.scrub(err.Error()))
	}
	defer resp.Body.Close()
	var reply struct {
		Ok          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&reply); err != nil {
		return fmt.Errorf("telegram %s: HTTP %d", method, resp.StatusCode)
	}
	if !reply.Ok {
		return fmt.Errorf("telegram %s: %s", method, b.scrub(reply.Description))
	}
	if out != nil && len(reply.Result) > 0 {
		return json.Unmarshal(reply.Result, out)
	}
	return nil
}

type chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type update struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		MessageID int64  `json:"message_id"`
		Text      string `json:"text"`
		Chat      chat   `json:"chat"`
		From      *struct {
			ID int64 `json:"id"`
		} `json:"from"`
		Document *struct {
			FileID   string `json:"file_id"`
			FileName string `json:"file_name"`
			FileSize int64  `json:"file_size"`
		} `json:"document"`
	} `json:"message"`
	Callback *struct {
		ID   string `json:"id"`
		Data string `json:"data"`
		From struct {
			ID int64 `json:"id"`
		} `json:"from"`
		Message *struct {
			MessageID int64 `json:"message_id"`
			Chat      chat  `json:"chat"`
		} `json:"message"`
	} `json:"callback_query"`
}

func (b *bot) run(ctx context.Context) {
	var me struct {
		Username string `json:"username"`
	}
	if err := b.call(ctx, "getMe", map[string]any{}, &me); err != nil {
		logger.Warning("telegram bot: getMe failed (check the token and proxy): ", err)
	} else {
		logger.Info("telegram bot: running as @", me.Username)
	}
	b.registerCommands(ctx)
	go b.watch(ctx)
	b.announce(ctx)

	var offset int64
	backoff := time.Second
	for ctx.Err() == nil {
		var updates []update
		err := b.call(ctx, "getUpdates", map[string]any{
			"offset":          offset,
			"timeout":         pollTimeoutSeconds,
			"allowed_updates": []string{"message", "callback_query"},
		}, &updates)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Warning("telegram bot: poll failed: ", err)
			if !sleep(ctx, backoff) {
				return
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		for _, u := range updates {
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
			b.safeHandle(ctx, u)
		}
	}
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func (b *bot) safeHandle(ctx context.Context, u update) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("telegram bot: handler panic: ", r)
		}
	}()
	b.handle(ctx, u)
}

// send delivers text as HTML, split on line boundaries to stay under Telegram's
// message size limit. The keyboard, if any, goes on the last part.
func (b *bot) send(ctx context.Context, chatID int64, text string) {
	b.sendKeyboard(ctx, chatID, text, nil)
}

func (b *bot) sendKeyboard(ctx context.Context, chatID int64, text string, keyboard [][]button) {
	parts := splitMessage(text, maxMessageRunes)
	for i, chunk := range parts {
		params := map[string]any{
			"chat_id":                  chatID,
			"text":                     chunk,
			"parse_mode":               "HTML",
			"disable_web_page_preview": true,
		}
		if keyboard != nil && i == len(parts)-1 {
			params["reply_markup"] = map[string]any{"inline_keyboard": keyboard}
		}
		if err := b.call(ctx, "sendMessage", params, nil); err != nil && ctx.Err() == nil {
			logger.Warning("telegram bot: send failed: ", err)
			return
		}
	}
}

// edit rewrites a message in place, which keeps button presses from piling up
// new messages. An unchanged message is not an error worth logging.
func (b *bot) edit(ctx context.Context, chatID, messageID int64, text string, keyboard [][]button) {
	parts := splitMessage(text, maxMessageRunes)
	if len(parts) == 0 {
		return
	}
	params := map[string]any{
		"chat_id":                  chatID,
		"message_id":               messageID,
		"text":                     parts[0],
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	}
	if keyboard != nil {
		params["reply_markup"] = map[string]any{"inline_keyboard": keyboard}
	}
	if err := b.call(ctx, "editMessageText", params, nil); err != nil && ctx.Err() == nil && !strings.Contains(err.Error(), "not modified") {
		logger.Warning("telegram bot: edit failed: ", err)
	}
}

func (b *bot) answer(ctx context.Context, callbackID, text string) {
	_ = b.call(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": callbackID, "text": text}, nil)
}

// upload sends a file or photo as multipart form data.
func (b *bot) upload(ctx context.Context, method, field string, chatID int64, filename, caption string, data []byte) error {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("chat_id", strconv.FormatInt(chatID, 10))
	if caption != "" {
		_ = w.WriteField("caption", caption)
		_ = w.WriteField("parse_mode", "HTML")
	}
	part, err := w.CreateFormFile(field, filename)
	if err != nil {
		return err
	}
	if _, err := part.Write(data); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/bot"+b.cfg.Token+"/"+method, &body)
	if err != nil {
		return errors.New(b.scrub(err.Error()))
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	client := *b.client
	client.Timeout = 2 * time.Minute
	resp, err := client.Do(req)
	if err != nil {
		return errors.New(b.scrub(err.Error()))
	}
	defer resp.Body.Close()
	var reply struct {
		Ok          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&reply); err != nil || !reply.Ok {
		return fmt.Errorf("telegram %s failed: %s", method, b.scrub(reply.Description))
	}
	return nil
}

func (b *bot) broadcast(ctx context.Context, text string) {
	for _, id := range b.cfg.Admins {
		b.send(ctx, id, text)
	}
}

func splitMessage(text string, limit int) []string {
	if text == "" {
		return nil
	}
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.TrimRight(string(cur), "\n"))
			cur = cur[:0]
		}
	}
	for _, line := range strings.SplitAfter(text, "\n") {
		runes := []rune(line)
		for len(runes) > limit {
			flush()
			out = append(out, string(runes[:limit]))
			runes = runes[limit:]
		}
		if len(cur)+len(runes) > limit {
			flush()
		}
		cur = append(cur, runes...)
	}
	flush()
	return out
}

// download fetches a file a user sent to the bot (limited to 2 MiB).
func (b *bot) download(ctx context.Context, fileID string) ([]byte, error) {
	var f struct {
		Path string `json:"file_path"`
		Size int64  `json:"file_size"`
	}
	if err := b.call(ctx, "getFile", map[string]any{"file_id": fileID}, &f); err != nil {
		return nil, err
	}
	if f.Path == "" || f.Size > 2<<20 {
		return nil, errors.New("file too large or unavailable")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/file/bot"+b.cfg.Token+"/"+f.Path, nil)
	if err != nil {
		return nil, errors.New(b.scrub(err.Error()))
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, errors.New(b.scrub(err.Error()))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram file: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 2<<20))
}
