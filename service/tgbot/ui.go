package tgbot

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const rule = "━━━━━━━━━━━━━━━━"

// tr picks the Persian or English text; used for the strings of the panel
// screens so each sits next to the code that shows it.
func (b *bot) tr(fa, en string) string {
	if b.cfg.Lang == "en" {
		return en
	}
	return fa
}

func (b *bot) header(icon, title string) string {
	return "<b>" + icon + " " + title + "</b>\n" + rule
}

func bar(pct, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	n := (pct*width + 50) / 100
	return strings.Repeat("▰", n) + strings.Repeat("▱", width-n)
}

func pctOf(cur, total uint64) int {
	if total == 0 {
		return 0
	}
	return int(cur * 100 / total)
}

// spark draws a tiny chart from non-negative values.
func spark(vals []int64) string {
	levels := []rune("▁▂▃▄▅▆▇█")
	var max int64
	for _, v := range vals {
		if v > max {
			max = v
		}
	}
	var sb strings.Builder
	for _, v := range vals {
		i := 0
		if max > 0 && v > 0 {
			i = int(v * int64(len(levels)-1) / max)
			if i == 0 {
				i = 1
			}
		}
		sb.WriteRune(levels[i])
	}
	return sb.String()
}

func onOff(on bool) string {
	if on {
		return "✅"
	}
	return "⬜"
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// pageSlice returns the items of one page and the clamped page number.
func pageSlice(total, page, size int) (from, to, clamped, pages int) {
	pages = (total + size - 1) / size
	if pages == 0 {
		pages = 1
	}
	if page < 0 {
		page = 0
	}
	if page >= pages {
		page = pages - 1
	}
	from = page * size
	to = from + size
	if to > total {
		to = total
	}
	return from, to, page, pages
}

// pager renders "◀ 2/5 ▶" buttons; prefix is the callback data up to the page.
func pager(prefix string, page, pages int) []button {
	if pages <= 1 {
		return nil
	}
	row := []button{}
	if page > 0 {
		row = append(row, button{Text: "◀️", Data: fmt.Sprintf("%s:%d", prefix, page-1)})
	}
	row = append(row, button{Text: fmt.Sprintf("%d / %d", page+1, pages), Data: "noop"})
	if page < pages-1 {
		row = append(row, button{Text: "▶️", Data: fmt.Sprintf("%s:%d", prefix, page+1)})
	}
	return row
}

// navRow is the bottom row of most screens.
func (b *bot) navRow(back string) []button {
	if back == "" {
		return b.menuRow()
	}
	return []button{{Text: b.tr("⬅️ بازگشت", "⬅️ Back"), Data: back}, {Text: b.tr("🏠 منو", "🏠 Menu"), Data: "m:menu"}}
}

func (b *bot) cancelRow() []button {
	return []button{{Text: b.tr("✖️ انصراف", "✖️ Cancel"), Data: "x:cancel"}}
}

func rows2(btns []button) [][]button {
	var out [][]button
	for i := 0; i < len(btns); i += 2 {
		end := i + 2
		if end > len(btns) {
			end = len(btns)
		}
		out = append(out, btns[i:end])
	}
	return out
}

// pending is an input the bot is waiting for from an administrator: the next
// text message (or uploaded file) answers it.
type pending struct {
	kind   string
	id     uint
	key    string
	chatID int64
	msgID  int64 // the panel message to refresh afterwards
	back   string
	data   map[string]string
	at     time.Time
}

type pendingStore struct {
	mu sync.Mutex
	m  map[int64]*pending
}

const pendingTTL = 15 * time.Minute

func (s *pendingStore) set(chatID int64, p *pending) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[int64]*pending{}
	}
	p.chatID, p.at = chatID, time.Now()
	s.m[chatID] = p
}

func (s *pendingStore) get(chatID int64) *pending {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.m[chatID]
	if p != nil && time.Since(p.at) > pendingTTL {
		delete(s.m, chatID)
		return nil
	}
	return p
}

func (s *pendingStore) clear(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, chatID)
}

func (b *bot) deleteMessage(ctx context.Context, chatID, messageID int64) {
	if messageID == 0 {
		return
	}
	_ = b.call(ctx, "deleteMessage", map[string]any{"chat_id": chatID, "message_id": messageID}, nil)
}
