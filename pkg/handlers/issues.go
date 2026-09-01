package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (h *Handler) HandleIssueByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/issues/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	issueID := parts[0]
	action := parts[1]
	db, _, err := h.DBM.FindIssueDB(issueID)
	if err != nil {
		WriteJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	switch action {
	case "description":
		var body struct {
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.Description == "" {
			http.Error(w, "description is required", http.StatusBadRequest)
			return
		}
		if err := db.UpdateIssueDescription(issueID, body.Description); err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		h.Hub.BroadcastEvent("issue_updated", map[string]interface{}{"issue_id": issueID, "description": body.Description})
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "description": body.Description})

	default:
		http.Error(w, "unknown action: "+action, http.StatusBadRequest)
	}
}
