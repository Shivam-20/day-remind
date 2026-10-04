package web

import (
	"net/http"

	"github.com/shivam-20/day-remind/internal/auth"
	"github.com/shivam-20/day-remind/internal/storage"
)

type totpEnableRequest struct {
	Code string `json:"code"`
}

func (s *Server) handleTOTPStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.db.GetTOTPStatus(r.Context(), userIDFromContext(r.Context()))
	if writeStorageErr(w, err, "not found") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": status.Enabled})
}

func (s *Server) handleTOTPSetup(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	secret, err := s.db.BeginTOTPSetup(r.Context(), userID)
	if err == storage.ErrTOTPAlreadyEnabled {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save secret")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"secret":           secret,
		"provisioning_uri": auth.ProvisioningURI(secret, user.Email),
	})
}

func (s *Server) handleTOTPEnable(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var body totpEnableRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.db.EnableTOTP(r.Context(), userID, body.Code); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true})
}

func (s *Server) handleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var body totpEnableRequest
	_ = decodeJSON(r, &body)
	if err := s.db.DisableTOTP(r.Context(), userID, body.Code); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
}
