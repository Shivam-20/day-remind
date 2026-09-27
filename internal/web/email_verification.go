package web

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/storage"
)

// appBaseURL returns the public origin the SPA lives at, including the
// APP_BASE_PATH prefix when deployed behind the OCI/Caddy path proxy.
// WEBHOOK_BASE_URL wins when set so links survive proxy hops.
func (s *Server) appBaseURL(r *http.Request) string {
	if base := strings.TrimSuffix(s.cfg.WebhookBaseURL, "/"); base != "" {
		return base
	}
	base := publicBaseURL(r)
	if s.cfg.BasePath != "" {
		base += "/" + s.cfg.BasePath
	}
	return base
}

// sendVerificationEmail issues a token (if needed) and emails the verify link.
// It is best-effort: signup must not fail because email delivery is down.
func (s *Server) sendVerificationEmail(r *http.Request, userID uuid.UUID, email string) {
	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil || user.EmailVerifiedAt != nil {
		return
	}
	token, err := s.db.CreateEmailVerificationToken(r.Context(), userID)
	if err != nil || token == "" {
		return
	}
	delivery, err := s.effectiveDelivery(r)
	if err != nil || !notify.ChannelAvailability(delivery)[storage.ChannelEmail] {
		return
	}
	link := s.appBaseURL(r) + "/#/verify-email?token=" + token
	_ = notify.SendEmail(r.Context(), delivery, strings.TrimSpace(strings.ToLower(email)),
		"Verify your Day Remind email",
		"<p>Welcome to Day Remind! Click to verify your email address:</p>"+
			"<p><a href=\""+link+"\">"+link+"</a></p>"+
			"<p>Link expires in 24 hours. If you didn't create this account, ignore this email.</p>")
}

func (s *Server) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.db.ConsumeEmailVerificationToken(r.Context(), body.Token); err == storage.ErrNotFound {
		writeError(w, http.StatusBadRequest, "invalid or expired verification link")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not verify email")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleResendVerification(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if user.EmailVerifiedAt != nil {
		writeError(w, http.StatusBadRequest, "email is already verified")
		return
	}
	delivery, err := s.effectiveDelivery(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load delivery settings")
		return
	}
	if !notify.ChannelAvailability(delivery)[storage.ChannelEmail] {
		writeError(w, http.StatusBadRequest, "email delivery is not configured — ask an admin to enable it")
		return
	}
	s.sendVerificationEmail(r, userID, user.Email)
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}
