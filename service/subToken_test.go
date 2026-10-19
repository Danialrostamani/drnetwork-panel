package service

import (
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// An edit keeps the client's token whatever the form sent; only ResetSubToken
// moves the link, and the old one is gone after it.
func TestSubTokenSurvivesEditsAndResets(t *testing.T) {
	db := clientTestDB(t)
	s := &ClientService{}
	c := createClient(t, db, &model.Client{Name: "tok", Enable: true})
	token := reload(t, db, c.Id).SubToken
	if len(token) != model.SubTokenLength {
		t.Fatalf("token %q", token)
	}

	for _, sent := range []string{"", "abcdefghijklmnopqrstuvwx"} {
		edited := &model.Client{Id: c.Id, Name: "renamed", Enable: true, SubToken: sent}
		s.preserveServerOwnedFields(db, edited)
		if edited.SubToken != token {
			t.Errorf("an edit sending %q moved the token to %q", sent, edited.SubToken)
		}
	}

	set := &SettingService{}
	link, err := set.ClientSubURL("tok", "panel.example.org")
	if err != nil || !strings.HasSuffix(link, "/"+token) || strings.HasSuffix(link, "/tok") {
		t.Fatalf("link %q, %v", link, err)
	}

	fresh, err := s.ResetSubToken(c.Id, "admin")
	if err != nil || len(fresh) != model.SubTokenLength || fresh == token {
		t.Fatalf("reset gave %q, %v", fresh, err)
	}
	if got := reload(t, db, c.Id).SubToken; got != fresh {
		t.Fatalf("stored %q, returned %q", got, fresh)
	}
	var logged int64
	db.Model(model.Changes{}).Where("action = ? AND actor = ?", "subtoken", "admin").Count(&logged)
	if logged != 1 {
		t.Errorf("%d change rows for the reset", logged)
	}
	if _, err := s.ResetSubToken(c.Id+100, "admin"); err == nil {
		t.Error("a missing client got a token")
	}
}

func TestSubTokensAreRandomAndLinkSafe(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		tok := model.NewSubToken()
		if len(tok) != model.SubTokenLength || strings.Trim(tok, "abcdefghijklmnopqrstuvwxyz0123456789") != "" {
			t.Fatalf("token %q", tok)
		}
		if seen[tok] {
			t.Fatalf("token %q twice", tok)
		}
		seen[tok] = true
	}
}
