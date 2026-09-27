package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestIsGateway(t *testing.T) {
	cases := []struct {
		name string
		d    DeliveryConfig
		want bool
	}{
		{"empty", DeliveryConfig{}, false},
		{"token only", DeliveryConfig{TelegramBotToken: "123:abc"}, false},
		{"gateway url only", DeliveryConfig{TelegramGatewayURL: "http://gw"}, false},
		{"both gateway", DeliveryConfig{TelegramGatewayURL: "http://gw", TelegramGatewayAPIToken: "tgw_123"}, true},
	}
	for _, c := range cases {
		if got := IsGateway(c.d); got != c.want {
			t.Errorf("%s: IsGateway = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestCallTelegramRoutesThroughGateway(t *testing.T) {
	var gotMethod, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = strings.TrimPrefix(r.URL.Path, "/v1/telegram/")
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":42}}`))
	}))
	defer srv.Close()

	d := DeliveryConfig{
		TelegramGatewayURL:      srv.URL,
		TelegramGatewayAPIToken: "tgw_secret",
	}
	result, err := callTelegram(context.Background(), d, "sendMessage", map[string]any{
		"chat_id": "123", "text": "hello",
	})
	if err != nil {
		t.Fatalf("callTelegram: %v", err)
	}
	if gotMethod != "sendMessage" {
		t.Errorf("gateway method = %q, want %q", gotMethod, "sendMessage")
	}
	if gotAuth != "Bearer tgw_secret" {
		t.Errorf("auth header = %q, want %q", gotAuth, "Bearer tgw_secret")
	}
	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if m["message_id"] != float64(42) {
		t.Errorf("result message_id = %v, want 42", m["message_id"])
	}
}

func TestSetTelegramWebhookNoOpInGatewayMode(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
	}))
	defer srv.Close()

	d := DeliveryConfig{
		TelegramGatewayURL:      srv.URL,
		TelegramGatewayAPIToken: "token",
	}
	if err := SetTelegramWebhook(context.Background(), d, "https://example.com/wh", "secret"); err != nil {
		t.Fatalf("SetTelegramWebhook (gateway): %v", err)
	}
	if hit {
		t.Error("SetTelegramWebhook should not hit the server in gateway mode")
	}
	if err := DeleteTelegramWebhook(context.Background(), d); err != nil {
		t.Fatalf("DeleteTelegramWebhook (gateway): %v", err)
	}
	if hit {
		t.Error("DeleteTelegramWebhook should not hit the server in gateway mode")
	}
}

func TestGetGatewayInboundUpdates(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		// Return two gateway inbound events with wrapped Telegram payloads.
		_, _ = w.Write([]byte(`{"ok":true,"result":[
			{"id":101,"update_id":1,"payload":{"update_id":1,"message":{"message_id":10,"chat":{"id":123},"text":"hello"}}},
			{"id":102,"update_id":2,"payload":{"update_id":2,"callback_query":{"id":"cb1","message":{"message_id":20,"chat":{"id":123}},"data":"done:abc"}}}
		]}`))
	}))
	defer srv.Close()

	d := DeliveryConfig{
		TelegramGatewayURL:      srv.URL,
		TelegramGatewayAPIToken: "tgw_test",
	}
	updates, ids, err := GetGatewayInboundUpdates(context.Background(), d, 0, 50)
	if err != nil {
		t.Fatalf("GetGatewayInboundUpdates: %v", err)
	}
	if len(updates) != 2 {
		t.Fatalf("expected 2 updates, got %d", len(updates))
	}
	if updates[0].Message == nil || updates[0].Message.Text != "hello" {
		t.Errorf("update[0] text = %v, want %q", unwrap(updates[0].Message), "hello")
	}
	if updates[1].CallbackQuery == nil || updates[1].CallbackQuery.Data != "done:abc" {
		t.Errorf("update[1] callback data = %v, want %q", unwrapCallback(updates[1].CallbackQuery), "done:abc")
	}
	if len(ids) != 2 || ids[0] != 101 || ids[1] != 102 {
		t.Errorf("ids = %v, want [101 102]", ids)
	}
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 HTTP call, got %d", atomic.LoadInt32(&callCount))
	}
}

// helpers for test error messages
func unwrap(m *TelegramMessage) string {
	if m == nil {
		return "<nil>"
	}
	return m.Text
}
func unwrapCallback(cb *TelegramCallback) string {
	if cb == nil {
		return "<nil>"
	}
	return cb.Data
}

func TestGetTelegramUpdatesGatewayDelegates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":[
			{"id":5,"update_id":1,"payload":{"update_id":1,"message":{"message_id":1,"chat":{"id":99},"text":"ping"}}}
		]}`))
	}))
	defer srv.Close()

	d := DeliveryConfig{
		TelegramGatewayURL:      srv.URL,
		TelegramGatewayAPIToken: "tgw_test",
	}
	updates, err := GetTelegramUpdates(context.Background(), d, 0, 30)
	if err != nil {
		t.Fatalf("GetTelegramUpdates (gateway): %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	if updates[0].Message == nil || updates[0].Message.Text != "ping" {
		t.Errorf("unexpected update: %+v", updates[0])
	}
}

func TestTelegramCallbackDataRoundTrip(t *testing.T) {
	cases := []struct {
		action, id, arg string
	}{
		{TelegramCallbackDone, "9c1f-abc", ""},
		{TelegramCallbackSnooze, "9c1f-abc", "1h"},
		{TelegramCallbackPause, "9c1f-abc", ""},
	}
	for _, c := range cases {
		action, id, arg := ParseTelegramCallbackData(TelegramCallbackData(c.action, c.id, c.arg))
		if action != c.action || id != c.id || arg != c.arg {
			t.Fatalf("round trip = (%q,%q,%q), want (%q,%q,%q)", action, id, arg, c.action, c.id, c.arg)
		}
	}
}

func TestParseTelegramCallbackDataEmpty(t *testing.T) {
	action, id, arg := ParseTelegramCallbackData("")
	if action != "" || id != "" || arg != "" {
		t.Fatalf("expected empty parts, got (%q,%q,%q)", action, id, arg)
	}
}

func TestEscapeTelegramHTML(t *testing.T) {
	got := EscapeTelegramHTML(`<b>a & b</b>`)
	want := `&lt;b&gt;a &amp; b&lt;/b&gt;`
	if got != want {
		t.Fatalf("EscapeTelegramHTML = %q, want %q", got, want)
	}
}

func TestTelegramReminderKeyboard(t *testing.T) {
	withLink := TelegramReminderKeyboard("abc", "https://x/#/reminders?open=abc")
	if len(withLink.InlineKeyboard) != 2 {
		t.Fatalf("expected 2 rows with deep link, got %d", len(withLink.InlineKeyboard))
	}
	first := withLink.InlineKeyboard[0]
	if len(first) != 3 || first[0].CallbackData != "done:abc" || first[1].CallbackData != "snooze:abc:1h" {
		t.Fatalf("unexpected first row: %+v", first)
	}
	withoutLink := TelegramReminderKeyboard("abc", "")
	if len(withoutLink.InlineKeyboard) != 1 {
		t.Fatalf("expected 1 row without deep link, got %d", len(withoutLink.InlineKeyboard))
	}
}
