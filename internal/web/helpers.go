package web

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/storage"
)

func parseURLUUID(w http.ResponseWriter, r *http.Request, param string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, param))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid "+param)
		return uuid.Nil, false
	}
	return id, true
}

func writeStorageErr(w http.ResponseWriter, err error, notFoundMsg string) bool {
	if err == nil {
		return false
	}
	switch err {
	case storage.ErrNotFound:
		writeError(w, http.StatusNotFound, notFoundMsg)
	case storage.ErrConflict:
		writeError(w, http.StatusConflict, "conflict")
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
	return true
}

func uuidStrings(ids []uuid.UUID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}

func parseOptionalUUID(raw string) (*uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func parseTagIDs(raw []string) ([]uuid.UUID, error) {
	if raw == nil {
		return nil, nil
	}
	out := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}
