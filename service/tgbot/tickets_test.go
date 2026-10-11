package tgbot

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

func voiceFrom(from int64, fileID string) update {
	var u update
	raw := `{"update_id":4,"message":{"message_id":6,"voice":{"file_id":` + mustJSON(fileID) + `},"chat":{"id":` + itoa(from) + `,"type":"private"},"from":{"id":` + itoa(from) + `,"first_name":"Buyer"}}}`
	_ = json.Unmarshal([]byte(raw), &u)
	return u
}

func to(msgs []sent, chatID int64) []sent {
	var out []sent
	for _, m := range msgs {
		if m.ChatID == chatID {
			out = append(out, m)
		}
	}
	return out
}

func TestSupportTickets(t *testing.T) {
	e, _ := shopEnv(t)
	s := &service.ShopService{}

	// The support button listens; the first message opens a ticket and
	// reaches the shop's administrators with the buttons to answer.
	e.press(buyer, "p:support")
	n := len(e.got())
	e.say(buyer, "My internet is slow")
	got := e.since(n)
	if txt := lastText(to(got, buyer), "sendMessage"); !strings.Contains(txt, "#1") {
		t.Fatalf("customer heard %q", txt)
	}
	admin := to(got, 42)
	if len(admin) == 0 || !strings.Contains(admin[0].Text, "My internet is slow") || !has(inlineData(admin), "q:tk:r:1") {
		t.Fatalf("support got %+v", admin)
	}

	// A photo and a voice message go on the same ticket, as media.
	n = len(e.got())
	e.b.handle(context.Background(), photoFrom(buyer, "PHOTO1"))
	e.b.handle(context.Background(), voiceFrom(buyer, "VOICE1"))
	got = to(e.since(n), 42)
	methods := map[string]bool{}
	for _, m := range got {
		methods[m.Method] = true
	}
	if !methods["sendPhoto"] || !methods["sendVoice"] {
		t.Fatalf("media to support: %+v", got)
	}
	if ts, _ := s.Tickets(buyer, false, 10); len(ts) != 1 {
		t.Fatalf("%d tickets", len(ts))
	}
	if msgs, _ := s.TicketMsgs(1, 10); len(msgs) != 3 || msgs[1].Kind != "photo" || msgs[1].FileId != "PHOTO1" || msgs[2].Kind != "voice" {
		t.Fatalf("ticket messages %+v", msgs)
	}

	// The answer goes back to the customer.
	e.press(42, "q:tk:r:1")
	n = len(e.got())
	e.say(42, "Restart the app, please")
	got = to(e.since(n), buyer)
	if len(got) == 0 || !strings.Contains(got[len(got)-1].Text, "Restart the app, please") || !has(inlineData(got), "p:tk:x:1") {
		t.Fatalf("customer got %+v", got)
	}
	if tk, _ := s.Ticket(1); tk.Status != model.TicketAnswered {
		t.Fatalf("ticket %+v", tk)
	}

	// Somebody else's ticket stays theirs, and customers have no support
	// buttons.
	if _, err := s.TicketClose(1, referrer); !errors.Is(err, service.ErrTicketNotFound) {
		t.Fatalf("closed by a stranger: %v", err)
	}
	n = len(e.got())
	e.press(referrer, "q:tk:v:1")
	if strings.Contains(lastText(e.since(n), "sendMessage", "editMessageText"), "slow") {
		t.Fatal("a customer read the support's view")
	}

	// Closed by the customer, the next message opens a new ticket.
	e.press(buyer, "p:tk:x:1")
	e.press(buyer, "p:support")
	e.say(buyer, "Another question")
	if ts, _ := s.Tickets(buyer, false, 10); len(ts) != 2 || ts[0].Status != model.TicketOpen {
		t.Fatalf("tickets %+v", ts)
	}

	// A flood waits for an answer.
	for i := 0; i < service.TicketMaxWaiting; i++ {
		_, _, _ = s.TicketWrite(referrer, "text", "hello", "")
	}
	if _, _, err := s.TicketWrite(referrer, "text", "hello", ""); !errors.Is(err, service.ErrTicketFlood) {
		t.Fatalf("flood: %v", err)
	}
}
