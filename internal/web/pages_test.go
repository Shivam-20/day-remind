package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shivam-20/day-remind/internal/config"
)

func TestLandingIsDefaultPage(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)

	landing := getBody(t, srv.URL+"/")
	if !strings.Contains(landing, "know what today is for") {
		t.Fatalf("GET / should serve the marketing landing, got %q", clip(landing))
	}
	if strings.Contains(landing, `id="app"`) {
		t.Fatal("GET / served the app shell instead of the landing")
	}

	portal := getBody(t, srv.URL+"/portal")
	if !strings.Contains(portal, `id="app"`) {
		t.Fatalf("GET /portal should serve the app, got %q", clip(portal))
	}
}

func getBody(t *testing.T, url string) string {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("%s: status %d", url, res.StatusCode)
	}
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func clip(s string) string {
	if len(s) > 180 {
		return s[:180]
	}
	return s
}
