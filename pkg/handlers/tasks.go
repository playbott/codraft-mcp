package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (h *Handler) HandleTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	project := r.URL.Query().Get("project")
	if project != "" {
		h.DBM.GetDefaultDB().UpsertSession(project)
	}
	db := h.DBM.GetDB(project)
	tasks, err := db.GetSummary("", project)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(tasks); err != nil {
		_ = err
	}
}

func (h *Handler) HandleTaskByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	taskID := parts[0]
	action := parts[1]

	db, _, err := h.DBM.FindTaskDB(taskID)
	if err != nil {
		WriteJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	var newStatus string
	switch action {
	case "approve":
		newStatus = "pending"
	case "reject":
		newStatus = "canceled"
	case "title":
		var body struct {
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.Title == "" {
			http.Error(w, "title is required", http.StatusBadRequest)
			return
		}
		if err := db.UpdateTaskTitle(taskID, body.Title); err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		h.Hub.BroadcastEvent("task_updated", map[string]interface{}{"task_id": taskID, "title": body.Title})
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "title": body.Title})
		return
	default:
		http.Error(w, "unknown action: "+action, http.StatusBadRequest)
		return
	}

	if err := db.UpdateTaskStatus(taskID, newStatus); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	h.Hub.BroadcastEvent("task_updated", map[string]interface{}{"task_id": taskID})
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "new_status": newStatus})
}
