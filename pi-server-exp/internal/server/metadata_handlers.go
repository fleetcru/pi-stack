package server

import (
	"encoding/json"
	"net/http"
)

type SessionMetadataUpdate struct {
	Project  *string            `json:"project"`
	Title    *string            `json:"title"`
	TaskType *string            `json:"taskType"`
	Owner    *string            `json:"owner"`
	Labels   *[]string          `json:"labels"`
	Metadata *map[string]string `json:"metadata"`
}

func (s *Server) updateSessionMetadata(w http.ResponseWriter, r *http.Request, id string) {
	var update SessionMetadataUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	spec, err := s.sessions.UpdateMetadata(id, update)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, spec)
}
