package web

import (
	"net/http"
	"strings"

	"github.com/shivam-20/day-remind/internal/auth"
	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/storage"
)

func (s *Server) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	token, err := s.db.CreatePasswordResetToken(r.Context(), body.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not process request")
		return
	}
	if token != "" {
		delivery, err := s.effectiveDelivery(r)
		if err == nil && notify.ChannelAvailability(delivery)[storage.ChannelEmail] {
			link := publicBaseURL(r) + "/#/reset-password?token=" + token
			_ = notify.SendEmail(r.Context(), delivery, strings.TrimSpace(strings.ToLower(body.Email)),
				"Reset your Day Remind password",
				"<p>Click to reset your password:</p><p><a href=\""+link+"\">"+link+"</a></p><p>Link expires in 1 hour.</p>")
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.db.ConsumePasswordResetToken(r.Context(), body.Token, hash); err == storage.ErrNotFound {
		writeError(w, http.StatusBadRequest, "invalid or expired token")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not reset password")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func publicBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.HasPrefix(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "localhost:8090"
	}
	return scheme + "://" + host
}
