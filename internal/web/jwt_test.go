package web

import (
	"net/http/httptest"
	"testing"

	"github.com/shivam-20/day-remind/internal/config"
)

// The home Caddy proxies these paths without forward_auth, so client-supplied
// X-Auth-* headers reach the app untouched and must be ignored.
func TestUserFromHeadersIgnoresUngatedPaths(t *testing.T) {
	s := &Server{cfg: config.Config{TrustAuthHeaders: true}}
	for _, path := range []string{
		"/api/webhooks/0b6c9c4e-1d7e-4a39-9f43-6f1b1c1d2e3f",
		"/api/webhooks/telegram",
		"/api/v1/reminders",
		"/api/health",
	} {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("X-Auth-Email", "someone@example.com")
		r.Header.Set("X-Auth-Role", "owner")
		if _, ok := s.userFromHeaders(r); ok {
			t.Errorf("%s: forged identity headers were trusted", path)
		}
	}
}

func TestIsUngatedPath(t *testing.T) {
	cases := map[string]bool{
		"/api/webhooks/x":   true,
		"/api/v1/reminders": true,
		"/api/health":       true,
		"/api/webhooks":     false, // gated by Caddy's /remind* handle
		"/api/me":           false,
		"/api/admin/stats":  false,
	}
	for path, want := range cases {
		if got := isUngatedPath(path); got != want {
			t.Errorf("isUngatedPath(%q) = %v, want %v", path, got, want)
		}
	}
}
