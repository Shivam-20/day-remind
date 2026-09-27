package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shivam-20/day-remind/internal/config"
)

// Non-secret fixture: generation tests only need a non-empty string; real
// Gemini keys live in env (GEMINI_API_KEY), never in source.
const geminiFixture = "fixture-gemini-value-not-usable"

func TestHandleGenerateMailWordingWithoutKey(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/api/me/mail-wordings/generate", strings.NewReader(`{"brief":"a friendly bill reminder"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleGenerateMailWording(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without an api key, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not configured") {
		t.Fatalf("unexpected error body: %s", rec.Body.String())
	}
}

func TestHandleGenerateMailWordingEmptyBrief(t *testing.T) {
	s := &Server{cfg: config.Config{GeminiAPIKey: geminiFixture}}
	req := httptest.NewRequest(http.MethodPost, "/api/me/mail-wordings/generate", strings.NewReader(`{"brief":"   "}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleGenerateMailWording(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty brief, got %d: %s", rec.Code, rec.Body.String())
	}
}
