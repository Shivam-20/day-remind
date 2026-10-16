package web

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/config"
	"github.com/shivam-20/day-remind/internal/delivery"
	"github.com/shivam-20/day-remind/internal/secret"
	"github.com/shivam-20/day-remind/internal/storage"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	testSessionKey = "web-test-session-key-at-least-32-chars!!"
	adminEmail     = "web-admin@example.com"
)

type harness struct {
	srv *httptest.Server
	db  *storage.DB
	box *secret.Box
}

// newHarness starts the real handler against a private database, so tests
// exercise routing, middleware and serialization exactly as production does.
func newHarness(t *testing.T, cfgTweak func(*config.Config)) *harness {
	t.Helper()
	raw := os.Getenv("DATABASE_URL")
	if raw == "" {
		t.Skip("DATABASE_URL not set")
	}
	name := "day_remind_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	u, err := neturl.Parse(raw)
	if err != nil {
		t.Fatalf("parse DATABASE_URL: %v", err)
	}
	u.Path = "/" + name

	db, err := storage.Open(u.String())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if c, cerr := mongo.Connect(ctx, options.Client().ApplyURI(u.String())); cerr == nil {
			_ = c.Database(name).Drop(ctx)
			_ = c.Disconnect(ctx)
		}
		_ = db.Close()
	})

	box, err := secret.NewBox(testSessionKey)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		SessionKey:  testSessionKey,
		AdminEmails: map[string]bool{adminEmail: true},
		// Point Telegram at a closed port: nothing must reach the real API.
		TelegramAPIBase: "http://127.0.0.1:1",
	}
	if cfgTweak != nil {
		cfgTweak(&cfg)
	}

	srv := httptest.NewServer(New(cfg, db, delivery.NewResolver(db, box, cfg.TelegramAPIBase), box).Handler())
	t.Cleanup(srv.Close)
	return &harness{srv: srv, db: db, box: box}
}

// client is an HTTP client with its own cookie jar, i.e. one logged-in session.
func (h *harness) client(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar, Timeout: 20 * time.Second}
}

func (h *harness) do(t *testing.T, c *http.Client, method, path string, body any) (int, string) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, h.srv.URL+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(out)
}

// signup creates an account and leaves the client logged in.
func (h *harness) signup(t *testing.T, c *http.Client, email string) {
	t.Helper()
	status, body := h.do(t, c, http.MethodPost, "/api/auth/signup", map[string]any{
		"email": email, "password": "password123",
		"display_name": "Test", "timezone": "UTC",
	})
	if status != http.StatusCreated {
		t.Fatalf("signup %s: %d %s", email, status, body)
	}
}

func (h *harness) admin(t *testing.T) *http.Client {
	t.Helper()
	c := h.client(t)
	h.signup(t, c, adminEmail)
	return c
}

func (h *harness) user(t *testing.T) *http.Client {
	t.Helper()
	c := h.client(t)
	h.signup(t, c, "web-user-"+uuid.NewString()+"@example.com")
	return c
}

func decode(t *testing.T, body string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	return out
}

// adminRoutes is every endpoint behind requireAdmin.
var adminRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/admin/stats"},
	{http.MethodGet, "/api/admin/users"},
	{http.MethodGet, "/api/admin/settings"},
	{http.MethodPatch, "/api/admin/settings"},
	{http.MethodPost, "/api/admin/settings/test"},
	{http.MethodPut, "/api/admin/templates"},
	{http.MethodPost, "/api/admin/templates/reset"},
	{http.MethodGet, "/api/admin/mail-templates"},
	{http.MethodPost, "/api/admin/mail-templates"},
	{http.MethodGet, "/api/admin/deliveries"},
}

// A non-admin must not even learn that the admin panel exists.
func TestAdminRoutesAreInvisibleToNonAdmins(t *testing.T) {
	h := newHarness(t, nil)
	user := h.user(t)

	for _, r := range adminRoutes {
		status, body := h.do(t, user, r.method, r.path, map[string]any{})
		if status != http.StatusNotFound {
			t.Errorf("%s %s as a normal user = %d, want 404\n%s", r.method, r.path, status, body)
		}
	}
}

func TestAdminRoutesRejectAnonymousCallers(t *testing.T) {
	h := newHarness(t, nil)
	anon := h.client(t)

	for _, r := range adminRoutes {
		status, _ := h.do(t, anon, r.method, r.path, map[string]any{})
		if status != http.StatusUnauthorized {
			t.Errorf("%s %s while logged out = %d, want 401", r.method, r.path, status)
		}
	}
}

func TestAdminEmailIsPromotedOnSignup(t *testing.T) {
	h := newHarness(t, nil)
	admin := h.admin(t)

	status, body := h.do(t, admin, http.MethodGet, "/api/admin/settings", nil)
	if status != http.StatusOK {
		t.Fatalf("admin settings = %d %s", status, body)
	}
}

const (
	secretPassword = "sup3r-s3cret-smtp-password"
	secretToken    = "987654:XYZ-secret-bot-token-value-here"
)

// The hard rule: no endpoint returns a stored credential.
func TestCredentialsNeverAppearInAnyResponse(t *testing.T) {
	h := newHarness(t, nil)
	admin := h.admin(t)

	status, body := h.do(t, admin, http.MethodPatch, "/api/admin/settings", map[string]any{
		"smtp": map[string]any{
			"enabled": true, "host": "smtp.example.com", "port": 587,
			"from": "reminders@example.com", "tls": "starttls", "password": secretPassword,
		},
		"telegram": map[string]any{"enabled": true, "bot_token": secretToken},
	})
	if status != http.StatusOK {
		t.Fatalf("save settings = %d %s", status, body)
	}
	// The response to the write itself must not echo them back.
	assertNoSecrets(t, "PATCH /api/admin/settings", body)

	for _, path := range []string{"/api/admin/settings", "/api/admin/users", "/api/admin/deliveries"} {
		_, got := h.do(t, admin, http.MethodGet, path, nil)
		assertNoSecrets(t, "GET "+path, got)
	}

	// And the admin view must still say a credential is stored.
	cfg := decode(t, mustGet(t, h, admin, "/api/admin/settings"))
	smtp := cfg["smtp"].(map[string]any)
	if smtp["password_set"] != true {
		t.Error("the panel should report that a password is stored")
	}
	tg := cfg["telegram"].(map[string]any)
	if tg["token_set"] != true {
		t.Error("the panel should report that a token is stored")
	}
	if hint, ok := tg["token_hint"].(string); !ok || !strings.HasPrefix(hint, "••••") {
		t.Errorf("expected a masked hint, got %v", tg["token_hint"])
	}
}

func TestUserOwnCredentialsAreNotReturned(t *testing.T) {
	h := newHarness(t, func(c *config.Config) { c.AllowPrivateSMTP = true })
	user := h.user(t)

	status, body := h.do(t, user, http.MethodPatch, "/api/me/notify/smtp", map[string]any{
		"enabled": true, "host": "localhost", "port": 1025, "tls": "none",
		"from": "me@example.com", "password": secretPassword,
	})
	if status != http.StatusOK {
		t.Fatalf("save own smtp = %d %s", status, body)
	}
	assertNoSecrets(t, "PATCH /api/me/notify/smtp", body)

	for _, path := range []string{"/api/me/notify", "/api/me", "/api/me/templates"} {
		_, got := h.do(t, user, http.MethodGet, path, nil)
		assertNoSecrets(t, "GET "+path, got)
	}
}

func assertNoSecrets(t *testing.T, where, body string) {
	t.Helper()
	for _, s := range []string{secretPassword, secretToken} {
		if strings.Contains(body, s) {
			t.Errorf("%s leaked a credential:\n%s", where, body)
		}
	}
	// Guard against a future field simply named "password" carrying a value.
	if strings.Contains(body, `"password":"`) || strings.Contains(body, `"bot_token":"`) {
		t.Errorf("%s returned a raw credential field:\n%s", where, body)
	}
}

func mustGet(t *testing.T, h *harness, c *http.Client, path string) string {
	t.Helper()
	status, body := h.do(t, c, http.MethodGet, path, nil)
	if status != http.StatusOK {
		t.Fatalf("GET %s = %d %s", path, status, body)
	}
	return body
}

// Blank means "keep the stored credential". Since it is never sent to the
// browser, a form that round-trips must not wipe it.
func TestOmittedPasswordKeepsTheStoredOne(t *testing.T) {
	h := newHarness(t, nil)
	admin := h.admin(t)

	base := map[string]any{
		"enabled": true, "host": "smtp.example.com", "port": 587,
		"from": "reminders@example.com", "tls": "starttls",
	}
	withPw := map[string]any{}
	for k, v := range base {
		withPw[k] = v
	}
	withPw["password"] = secretPassword
	if status, body := h.do(t, admin, http.MethodPatch, "/api/admin/settings", map[string]any{"smtp": withPw}); status != http.StatusOK {
		t.Fatalf("initial save = %d %s", status, body)
	}

	// Save again with no password field at all: the stored one must survive.
	base["host"] = "smtp2.example.com"
	if status, body := h.do(t, admin, http.MethodPatch, "/api/admin/settings", map[string]any{"smtp": base}); status != http.StatusOK {
		t.Fatalf("second save = %d %s", status, body)
	}
	cfg := decode(t, mustGet(t, h, admin, "/api/admin/settings"))
	smtp := cfg["smtp"].(map[string]any)
	if smtp["host"] != "smtp2.example.com" {
		t.Errorf("host was not updated: %v", smtp["host"])
	}
	if smtp["password_set"] != true {
		t.Error("omitting the password must not clear it")
	}

	// An explicit empty string clears it.
	base["password"] = ""
	if status, _ := h.do(t, admin, http.MethodPatch, "/api/admin/settings", map[string]any{"smtp": base}); status != http.StatusOK {
		t.Fatal("clearing save failed")
	}
	cfg = decode(t, mustGet(t, h, admin, "/api/admin/settings"))
	if cfg["smtp"].(map[string]any)["password_set"] != false {
		t.Error("an explicit empty password should clear the stored one")
	}
}

func TestSettingsValidationRejectsBadInput(t *testing.T) {
	h := newHarness(t, nil)
	admin := h.admin(t)

	cases := map[string]map[string]any{
		"no host":   {"enabled": true, "from": "a@example.com"},
		"no from":   {"enabled": true, "host": "smtp.example.com"},
		"bad from":  {"enabled": true, "host": "smtp.example.com", "from": "nope"},
		"bad tls":   {"enabled": true, "host": "smtp.example.com", "from": "a@example.com", "tls": "ssl"},
		"bad port":  {"enabled": true, "host": "smtp.example.com", "from": "a@example.com", "port": 99999},
		"zero port": {"enabled": true, "host": "smtp.example.com", "from": "a@example.com", "port": -1},
	}
	for name, smtp := range cases {
		status, _ := h.do(t, admin, http.MethodPatch, "/api/admin/settings", map[string]any{"smtp": smtp})
		if status != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400", name, status)
		}
	}
}

// A user-supplied mail server must not be able to point the process at
// arbitrary internal infrastructure.
func TestOwnSMTPTargetIsRestricted(t *testing.T) {
	h := newHarness(t, nil) // AllowPrivateSMTP is off

	user := h.user(t)
	status, body := h.do(t, user, http.MethodPatch, "/api/me/notify/smtp", map[string]any{
		"enabled": true, "host": "localhost", "port": 1025, "tls": "none", "from": "me@example.com",
	})
	if status != http.StatusBadRequest {
		t.Errorf("a loopback host should be refused, got %d %s", status, body)
	}
	if !strings.Contains(body, "private") {
		t.Errorf("the error should explain why: %s", body)
	}

	status, body = h.do(t, user, http.MethodPatch, "/api/me/notify/smtp", map[string]any{
		"enabled": true, "host": "smtp.example.com", "port": 8080, "from": "me@example.com",
	})
	if status != http.StatusBadRequest {
		t.Errorf("a non-mail port should be refused, got %d %s", status, body)
	}

	// With the development flag on, loopback is allowed again.
	h2 := newHarness(t, func(c *config.Config) { c.AllowPrivateSMTP = true })
	u2 := h2.user(t)
	if status, body := h2.do(t, u2, http.MethodPatch, "/api/me/notify/smtp", map[string]any{
		"enabled": true, "host": "localhost", "port": 1025, "tls": "none", "from": "me@example.com",
	}); status != http.StatusOK {
		t.Errorf("ALLOW_PRIVATE_SMTP should permit a local catcher, got %d %s", status, body)
	}
}

func TestTemplateValidationNamesTheField(t *testing.T) {
	h := newHarness(t, nil)
	admin := h.admin(t)

	status, body := h.do(t, admin, http.MethodPut, "/api/admin/templates", map[string]any{
		"email_subject": "ok {{.Title}}", "email_html": "{{.Title",
		"email_text": "", "telegram_text": "", "telegram_parse_mode": "",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("a broken template should be refused, got %d %s", status, body)
	}
	if !strings.Contains(body, "email_html") {
		t.Errorf("the error should name the offending field: %s", body)
	}

	// A valid set is accepted.
	if status, body := h.do(t, admin, http.MethodPut, "/api/admin/templates", map[string]any{
		"email_subject": "ok {{.Title}}", "email_html": "<b>{{.Title}}</b>",
		"email_text": "{{.Title}}", "telegram_text": "{{.Title}}", "telegram_parse_mode": "plain",
	}); status != http.StatusOK {
		t.Errorf("a valid template set was refused: %d %s", status, body)
	}
}

func TestTemplatePreviewRendersWithoutSaving(t *testing.T) {
	h := newHarness(t, nil)
	admin := h.admin(t)

	status, body := h.do(t, admin, http.MethodPost, "/api/admin/templates/preview", map[string]any{
		"email_subject": "PREVIEW {{.Title}}", "email_html": "<b>{{.Title}}</b>",
		"email_text": "{{.Title}}", "telegram_text": "{{.Title}}", "telegram_parse_mode": "plain",
	})
	if status != http.StatusOK {
		t.Fatalf("preview = %d %s", status, body)
	}
	out := decode(t, body)
	due := out["due"].(map[string]any)
	if !strings.HasPrefix(due["subject"].(string), "PREVIEW ") {
		t.Errorf("preview did not use the submitted template: %v", due["subject"])
	}
	if out["advance"] == nil {
		t.Error("preview should cover the advance notice too")
	}

	// Nothing was persisted.
	cfg := decode(t, mustGet(t, h, admin, "/api/admin/settings"))
	if got := cfg["templates"].(map[string]any)["email_subject"]; got != "" {
		t.Errorf("preview must not save: stored subject is %q", got)
	}
}

// A user's overrides layer onto the instance's, field by field.
func TestUserTemplatesInheritFromTheInstance(t *testing.T) {
	h := newHarness(t, nil)
	admin := h.admin(t)
	user := h.user(t)

	if status, body := h.do(t, admin, http.MethodPut, "/api/admin/templates", map[string]any{
		"email_subject": "INSTANCE {{.Title}}", "email_html": "", "email_text": "instance text",
		"telegram_text": "", "telegram_parse_mode": "",
	}); status != http.StatusOK {
		t.Fatalf("admin save = %d %s", status, body)
	}

	status, body := h.do(t, user, http.MethodPut, "/api/me/templates", map[string]any{
		"email_subject": "MINE {{.Title}}", "email_html": "", "email_text": "",
		"telegram_text": "", "telegram_parse_mode": "",
	})
	if status != http.StatusOK {
		t.Fatalf("user save = %d %s", status, body)
	}
	out := decode(t, body)
	own := out["own"].(map[string]any)
	inherited := out["inherited"].(map[string]any)
	if own["email_subject"] != "MINE {{.Title}}" {
		t.Errorf("own subject = %v", own["email_subject"])
	}
	if inherited["email_subject"] != "INSTANCE {{.Title}}" {
		t.Errorf("inherited subject = %v", inherited["email_subject"])
	}
	if inherited["email_text"] != "instance text" {
		t.Errorf("inherited text = %v", inherited["email_text"])
	}
}

func TestMailTemplateCreateUseAndDelete(t *testing.T) {
	h := newHarness(t, nil)
	user := h.user(t)

	status, body := h.do(t, user, http.MethodPost, "/api/me/mail-templates", map[string]any{
		"name":                "Bills",
		"email_subject":       "BILL {{.Title}}",
		"email_html":          "<p>{{.Title}}</p>",
		"email_css":           "p{color:teal}",
		"email_text":          "text {{.Title}}",
		"telegram_text":       "tg {{.Title}}",
		"telegram_parse_mode": "plain",
	})
	if status != http.StatusCreated {
		t.Fatalf("create = %d %s", status, body)
	}
	created := decode(t, body)
	id, _ := created["id"].(string)
	if id == "" || created["name"] != "Bills" {
		t.Fatalf("create payload = %v", created)
	}

	status, body = h.do(t, user, http.MethodPost, "/api/me/templates/preview", map[string]any{
		"email_html": "<p>{{.Title}}</p>",
		"email_css":  "p{color:teal}",
	})
	if status != http.StatusOK {
		t.Fatalf("preview = %d %s", status, body)
	}
	due := decode(t, body)["due"].(map[string]any)
	if !strings.Contains(due["html"].(string), "<style>p{color:teal}</style>") {
		t.Errorf("preview HTML did not include the CSS: %v", due["html"])
	}

	status, body = h.do(t, user, http.MethodPost, "/api/me/mail-templates/"+id+"/use", map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("use = %d %s", status, body)
	}
	used := decode(t, body)
	if used["active_id"] != id {
		t.Errorf("active_id = %v, want %s", used["active_id"], id)
	}

	own := decode(t, mustGet(t, h, user, "/api/me/templates"))["own"].(map[string]any)
	if own["email_subject"] != "BILL {{.Title}}" || own["email_css"] != "p{color:teal}" {
		t.Errorf("using a template did not copy the wording: %v", own)
	}

	if status, body = h.do(t, user, http.MethodDelete, "/api/me/mail-templates/"+id, nil); status != http.StatusNoContent {
		t.Fatalf("delete = %d %s", status, body)
	}
	left := decode(t, mustGet(t, h, user, "/api/me/mail-templates"))
	if left["active_id"] != "" {
		t.Errorf("deleted template is still active: %v", left["active_id"])
	}
	if n := len(left["templates"].([]any)); n != 0 {
		t.Errorf("library still has %d templates", n)
	}
}

// Switching on a channel nothing can deliver would be a silent no-op.
func TestEnablingAnUndeliverableChannelIsRefused(t *testing.T) {
	h := newHarness(t, nil)
	user := h.user(t)

	status, body := h.do(t, user, http.MethodPatch, "/api/me/notify", map[string]any{"email_enabled": true})
	if status != http.StatusConflict {
		t.Errorf("enabling email with no mail server = %d, want 409\n%s", status, body)
	}
	status, body = h.do(t, user, http.MethodPatch, "/api/me/notify", map[string]any{"telegram_enabled": true})
	if status != http.StatusConflict {
		t.Errorf("enabling telegram with no bot = %d, want 409\n%s", status, body)
	}
}

func TestLinkCodeRequiresAConfiguredBot(t *testing.T) {
	h := newHarness(t, nil)
	user := h.user(t)

	status, body := h.do(t, user, http.MethodPost, "/api/me/telegram/link-code", nil)
	if status != http.StatusConflict {
		t.Errorf("link code with no instance bot = %d, want 409\n%s", status, body)
	}
}

func TestDiscoverChatRequiresAToken(t *testing.T) {
	h := newHarness(t, nil)
	user := h.user(t)

	status, body := h.do(t, user, http.MethodPost, "/api/me/notify/telegram/discover", nil)
	if status != http.StatusConflict {
		t.Errorf("discover without a token = %d, want 409\n%s", status, body)
	}
	if !strings.Contains(body, "token") {
		t.Errorf("the error should say what is missing: %s", body)
	}
}

func TestTestNotificationNeedsAnEnabledChannel(t *testing.T) {
	h := newHarness(t, nil)
	user := h.user(t)

	status, body := h.do(t, user, http.MethodPost, "/api/me/notify/test", nil)
	if status != http.StatusConflict {
		t.Errorf("test with nothing enabled = %d, want 409\n%s", status, body)
	}
}
