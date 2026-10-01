package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func TestNodeCRUDProbeAndTokenRedaction(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "nodes.db")); err != nil {
		t.Fatal(err)
	}
	const token = "secret-node-token"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Token") != token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/app/apiv2/status":
			_, _ = w.Write([]byte(`{"success":true,"obj":{"cpu":12.5,"mem":{"current":10,"total":20},"sys":{"appVersion":"1.6.3"},"sbd":{"running":true,"version":"1.14.1"}}}`))
		case "/app/apiv2/onlines":
			_, _ = w.Write([]byte(`{"success":true,"obj":{"user":["alice"]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	svc := NodeService{}
	payload, _ := json.Marshal(model.Node{Name: "node-a", Enable: true, BaseUrl: srv.URL, WebPath: "app", Token: token})
	tx := database.GetDB().Begin()
	if err := svc.Save(tx, "new", payload); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}

	var stored model.Node
	if err := database.GetDB().First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Token != token || stored.WebPath != "/app/" {
		t.Fatalf("stored token/path = %q %q", stored.Token, stored.WebPath)
	}
	rows, err := svc.GetAll()
	if err != nil || len(rows) != 1 {
		t.Fatalf("GetAll = %#v, %v", rows, err)
	}
	if _, leaked := rows[0]["token"]; leaked || rows[0]["tokenSet"] != true {
		t.Fatalf("token leaked or tokenSet missing: %#v", rows[0])
	}

	svc.RefreshAll()
	status := svc.GetStatuses()[stored.Id]
	if status.State != "online" || status.AppVersion != "1.6.3" || status.CoreVersion != "1.14.1" {
		t.Fatalf("unexpected status: %#v", status)
	}

	edit, _ := json.Marshal(model.Node{Id: stored.Id, Name: stored.Name, Enable: true, BaseUrl: srv.URL, WebPath: "/app/", Desc: "edited"})
	tx = database.GetDB().Begin()
	if err := svc.Save(tx, "edit", edit); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().First(&stored, stored.Id).Error; err != nil || stored.Token != token {
		t.Fatalf("empty edit token did not preserve secret: %q, %v", stored.Token, err)
	}

	redacted := string(redactNodeToken(payload))
	if redacted == string(payload) || json.Valid([]byte(redacted)) == false {
		t.Fatalf("redaction failed: %s", redacted)
	}
	if _, err := svc.TestNode(json.RawMessage(`{"baseUrl":"ftp://bad","token":"x"}`)); err == nil {
		t.Fatal("invalid node URL accepted")
	}
}
