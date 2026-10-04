package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/shivam-20/day-remind/internal/config"
	"github.com/shivam-20/day-remind/internal/storage"
)

// Regression: in gateway mode the status handler returned without writing a
// body, so the admin Telegram page got null and never rendered.
func TestAdminTelegramStatusGatewayWritesBody(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := storage.Open(url, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/status" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer gw.Close()

	s := New(config.Config{
		TelegramUpdateMode:         "polling",
		TelegramGatewayURL:         gw.URL,
		TelegramGatewayAPIToken:    "tgw_fixture_not_a_real_token",
		TelegramGatewayInboundMode: "poll",
	}, db)
	rec := httptest.NewRecorder()
	s.handleAdminTelegramStatus(rec, httptest.NewRequest("GET", "/api/admin/telegram/status", nil))

	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("body %q is not JSON: %v", rec.Body.String(), err)
	}
	if out["uses_gateway"] != true || out["gateway_status"] == nil {
		t.Fatalf("unexpected status payload: %v", out)
	}
}
