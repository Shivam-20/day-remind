package web

import (
	"net/http"

	"github.com/shivam-20/day-remind/internal/storage"
)

type groupCreateRequest struct {
	Name string `json:"name"`
}

type groupUpdateRequest struct {
	Name string `json:"name"`
}

type groupInviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	items, err := s.db.ListGroups(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list groups")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, groupPayload(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": out})
}

func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var body groupCreateRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, err := s.db.CreateGroup(r.Context(), userID, body.Name)
	if writeStorageErr(w, err, "not found") {
		return
	}
	writeJSON(w, http.StatusCreated, groupPayload(item))
}

func (s *Server) handleGetGroup(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	item, err := s.db.GetGroup(r.Context(), userID, id)
	if writeStorageErr(w, err, "not found") {
		return
	}
	members, err := s.db.ListGroupMembers(r.Context(), userID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load members")
		return
	}
	out := groupPayload(item)
	memberOut := make([]map[string]any, 0, len(members))
	for _, m := range members {
		memberOut = append(memberOut, groupMemberPayload(m))
	}
	out["members"] = memberOut
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	var body groupUpdateRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, err := s.db.UpdateGroup(r.Context(), userID, id, body.Name)
	if writeStorageErr(w, err, "not found") {
		return
	}
	writeJSON(w, http.StatusOK, groupPayload(item))
}

func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	if err := s.db.DeleteGroup(r.Context(), userID, id); err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete group")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleInviteGroupMember(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	groupID, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	var body groupInviteRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, err := s.db.InviteGroupMember(r.Context(), userID, groupID, body.Email, body.Role)
	if err == storage.ErrConflict {
		writeError(w, http.StatusConflict, "member already invited")
		return
	}
	if writeStorageErr(w, err, "not found") {
		return
	}
	writeJSON(w, http.StatusCreated, groupMemberPayload(item))
}

func groupPayload(g storage.Group) map[string]any {
	return map[string]any{
		"id":       g.ID.String(),
		"owner_id": g.OwnerID.String(),
		"name":     g.Name,
	}
}

func groupMemberPayload(m storage.GroupMember) map[string]any {
	out := map[string]any{
		"group_id": m.GroupID.String(),
		"email":    m.Email,
		"role":     m.Role,
	}
	if m.UserID != nil {
		out["user_id"] = m.UserID.String()
	}
	return out
}
