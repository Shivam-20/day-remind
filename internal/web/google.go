package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shivam-20/day-remind/internal/storage"
)

func googleIntegrationPayload(g storage.GoogleIntegration) map[string]any {
	out := map[string]any{
		"connected":        true,
		"email":            g.Email,
		"calendar_enabled": g.CalendarEnabled,
		"gmail_enabled":    g.GmailEnabled,
		"calendar_paused":  g.CalendarPaused,
		"gmail_paused":     g.GmailPaused,
		"gmail_label_ids":  g.GmailLabelIDs,
		"updated_at":       g.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if g.LastCalendarSync != nil {
		out["last_calendar_sync"] = g.LastCalendarSync.UTC().Format(time.RFC3339)
	}
	if g.LastGmailSync != nil {
		out["last_gmail_sync"] = g.LastGmailSync.UTC().Format(time.RFC3339)
	}
	return out
}

func (s *Server) handleGetGoogleIntegration(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	g, err := s.db.GetGoogleIntegration(r.Context(), userID)
	if err == storage.ErrNotFound {
		writeJSON(w, http.StatusOK, map[string]any{"connected": false})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load integration")
		return
	}
	writeJSON(w, http.StatusOK, googleIntegrationPayload(g))
}

func (s *Server) handleGoogleConnect(w http.ResponseWriter, r *http.Request) {
	if s.cfg.GoogleClientID == "" || s.cfg.GoogleClientSecret == "" {
		writeError(w, http.StatusServiceUnavailable, "google oauth not configured")
		return
	}
	userID := userIDFromContext(r.Context())
	redirectURL := s.googleRedirectURL(r)
	q := url.Values{}
	q.Set("client_id", s.cfg.GoogleClientID)
	q.Set("redirect_uri", redirectURL)
	q.Set("response_type", "code")
	q.Set("scope", strings.Join([]string{
		"openid", "email",
		"https://www.googleapis.com/auth/calendar",
		"https://www.googleapis.com/auth/gmail.readonly",
	}, " "))
	q.Set("access_type", "offline")
	q.Set("prompt", "consent")
	q.Set("state", userID.String())
	authURL := "https://accounts.google.com/o/oauth2/v2/auth?" + q.Encode()
	writeJSON(w, http.StatusOK, map[string]string{"redirect_url": authURL})
}

func (s *Server) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" {
		writeError(w, http.StatusBadRequest, "code is required")
		return
	}
	if state != userID.String() {
		writeError(w, http.StatusBadRequest, "invalid state")
		return
	}
	if s.cfg.GoogleClientID == "" || s.cfg.GoogleClientSecret == "" {
		writeError(w, http.StatusServiceUnavailable, "google oauth not configured")
		return
	}
	token, email, err := exchangeGoogleCode(r, s.cfg.GoogleClientID, s.cfg.GoogleClientSecret, s.googleRedirectURL(r), code)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	g, err := s.db.SaveGoogleIntegration(r.Context(), storage.GoogleIntegration{
		UserID:          userID,
		Email:           email,
		RefreshToken:    token,
		CalendarEnabled: true,
		GmailEnabled:    false,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save integration")
		return
	}
	writeJSON(w, http.StatusOK, googleIntegrationPayload(g))
}

func (s *Server) handleGoogleDisconnect(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	if err := s.db.DeleteGoogleIntegration(r.Context(), userID); err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not connected")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not disconnect")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

func (s *Server) handleGoogleSync(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	now := time.Now().UTC()
	if err := s.db.UpdateGoogleCalendarSync(r.Context(), userID, now); err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not connected")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not sync")
		return
	}
	_ = s.db.UpdateGoogleGmailSync(r.Context(), userID, now)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":             "synced",
		"last_calendar_sync": now.Format(time.RFC3339),
		"last_gmail_sync":    now.Format(time.RFC3339),
	})
}

func (s *Server) handleGoogleCalendarPause(w http.ResponseWriter, r *http.Request) {
	s.setGoogleCalendarPaused(w, r, true)
}

func (s *Server) handleGoogleCalendarResume(w http.ResponseWriter, r *http.Request) {
	s.setGoogleCalendarPaused(w, r, false)
}

func (s *Server) handleGoogleGmailPause(w http.ResponseWriter, r *http.Request) {
	s.setGoogleGmailPaused(w, r, true)
}

func (s *Server) handleGoogleGmailResume(w http.ResponseWriter, r *http.Request) {
	s.setGoogleGmailPaused(w, r, false)
}

func (s *Server) setGoogleCalendarPaused(w http.ResponseWriter, r *http.Request, paused bool) {
	userID := userIDFromContext(r.Context())
	g, err := s.db.GetGoogleIntegration(r.Context(), userID)
	if writeStorageErr(w, err, "not connected") {
		return
	}
	g.CalendarPaused = paused
	out, err := s.db.SaveGoogleIntegration(r.Context(), g)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update integration")
		return
	}
	writeJSON(w, http.StatusOK, googleIntegrationPayload(out))
}

func (s *Server) setGoogleGmailPaused(w http.ResponseWriter, r *http.Request, paused bool) {
	userID := userIDFromContext(r.Context())
	g, err := s.db.GetGoogleIntegration(r.Context(), userID)
	if writeStorageErr(w, err, "not connected") {
		return
	}
	g.GmailPaused = paused
	out, err := s.db.SaveGoogleIntegration(r.Context(), g)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update integration")
		return
	}
	writeJSON(w, http.StatusOK, googleIntegrationPayload(out))
}

type gmailSettingsRequest struct {
	LabelIDs     []string `json:"label_ids"`
	GmailEnabled *bool    `json:"gmail_enabled"`
}

func (s *Server) handleGetGmailSettings(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	g, err := s.db.GetGoogleIntegration(r.Context(), userID)
	if writeStorageErr(w, err, "not connected") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"label_ids":     g.GmailLabelIDs,
		"gmail_enabled": g.GmailEnabled,
		"gmail_paused":  g.GmailPaused,
	})
}

func (s *Server) handleUpdateGmailSettings(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var body gmailSettingsRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	g, err := s.db.GetGoogleIntegration(r.Context(), userID)
	if writeStorageErr(w, err, "not connected") {
		return
	}
	if body.LabelIDs != nil {
		g.GmailLabelIDs = body.LabelIDs
	}
	if body.GmailEnabled != nil {
		g.GmailEnabled = *body.GmailEnabled
	}
	out, err := s.db.SaveGoogleIntegration(r.Context(), g)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update settings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"label_ids":     out.GmailLabelIDs,
		"gmail_enabled": out.GmailEnabled,
		"gmail_paused":  out.GmailPaused,
	})
}

func (s *Server) googleRedirectURL(r *http.Request) string {
	if s.cfg.GoogleRedirectURL != "" {
		return s.cfg.GoogleRedirectURL
	}
	scheme := "http"
	if strings.HasPrefix(r.Header.Get("X-Forwarded-Proto"), "https") || r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/api/integrations/google/callback"
}

func exchangeGoogleCode(r *http.Request, clientID, clientSecret, redirectURL, code string) (refreshToken, email string, err error) {
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("redirect_uri", redirectURL)
	form.Set("grant_type", "authorization_code")
	resp, err := http.PostForm("https://oauth2.googleapis.com/token", form)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", "", fmt.Errorf("google token exchange failed")
	}
	var tok struct {
		RefreshToken string `json:"refresh_token"`
		AccessToken  string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", "", err
	}
	if tok.RefreshToken == "" {
		return "", "", fmt.Errorf("missing refresh token")
	}
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	userResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return tok.RefreshToken, "", nil
	}
	defer userResp.Body.Close()
	var info struct {
		Email string `json:"email"`
	}
	_ = json.NewDecoder(userResp.Body).Decode(&info)
	return tok.RefreshToken, info.Email, nil
}
