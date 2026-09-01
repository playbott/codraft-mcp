package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (h *Handler) HandleWalkthroughs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/walkthroughs/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	planID := parts[0]
	action := parts[1]
	db, _, err := h.DBM.FindPlanDB(planID)
	if err != nil {
		WriteJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	switch action {
	case "add-comment":
		var body struct {
			TaskID string `json:"task_id"`
			Text   string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.Text == "" {
			http.Error(w, "text is required", http.StatusBadRequest)
			return
		}

		entityType := "plan"
		entityID := planID
		if body.TaskID != "" {
			entityType = "task"
			entityID = body.TaskID
		}
		cID, err := db.AddComment(entityType, entityID, "user", body.Text)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if body.TaskID != "" {
			db.UpdateTaskStatus(body.TaskID, "in_progress")
		}
		db.UpdatePlanStatus(planID, "in_progress")

		h.Hub.BroadcastEvent("comment_added", map[string]interface{}{
			"comment_id":  cID,
			"entity_type": entityType,
			"entity_id":   entityID,
		})
		h.Hub.BroadcastEvent("plan_updated", map[string]interface{}{"plan_id": planID})
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "comment_id": cID})

	case "report-issue":
		var body struct {
			TaskID      string `json:"task_id"`
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

		issueID, taskID, err := db.AutoHeal(planID, body.Description)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if body.TaskID != "" {
			db.UpdateIssueTaskID(issueID, body.TaskID)
		}

		h.Hub.BroadcastEvent("issue_reported", map[string]interface{}{
			"issue_id": issueID,
			"plan_id":  planID,
			"task_id":  taskID,
		})
		h.Hub.BroadcastEvent("plan_updated", map[string]interface{}{"plan_id": planID})
		WriteJSON(w, http.StatusOK, map[string]string{
			"status":   "ok",
			"issue_id": issueID,
			"task_id":  taskID,
		})

	case "folder":
		var body struct {
			Folder string `json:"folder"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if err := db.UpdatePlanFolder(planID, body.Folder); err != nil {
			WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		h.Hub.BroadcastEvent("plan_updated", map[string]interface{}{"plan_id": planID, "folder": body.Folder})
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "folder": body.Folder})

	default:
		http.Error(w, "unknown action: "+action, http.StatusBadRequest)
	}
}
