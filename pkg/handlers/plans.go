package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (h *Handler) HandlePlans(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/plans" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	project := r.URL.Query().Get("project")
	if project != "" {
		h.DBM.GetDefaultDB().UpsertSession(project)
	}
	db := h.DBM.GetDB(project)
	plans, err := db.GetPlans(project)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(plans)
}

func (h *Handler) HandlePlanByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/plans/")
	if id == "" {
		http.Error(w, "plan ID required", http.StatusBadRequest)
		return
	}

	parts := strings.SplitN(id, "/", 2)
	if len(parts) == 2 {
		h.handlePlanAction(w, r, parts[0], parts[1])
		return
	}
	id = parts[0]

	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	_, plan, err := h.DBM.FindPlanDB(id)
	if err != nil {
		WriteJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(plan)
}

func (h *Handler) handlePlanAction(w http.ResponseWriter, r *http.Request, planID, action string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	db, _, err := h.DBM.FindPlanDB(planID)
	if err != nil {
		WriteJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	if action == "folder" {
		var body struct {
			Folder string `json:"folder"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if err := db.SetPlanFolder(planID, body.Folder); err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		h.Hub.BroadcastEvent("plan_updated", map[string]interface{}{"plan_id": planID, "folder": body.Folder})
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "folder": body.Folder})
		return
	}

	var newStatus string
	switch action {
	case "approve":
		newStatus = "approved"
	case "reject":
		newStatus = "rejected"
	case "draft", "redraft":
		newStatus = "draft"
	case "cancel":
		newStatus = "canceled"
	case "hold":
		newStatus = "on_hold"
	case "resume":
		newStatus = "in_progress"
	case "complete":
		newStatus = "completed"
	default:
		http.Error(w, "unknown action: "+action, http.StatusBadRequest)
		return
	}

	if err := db.UpdatePlanStatus(planID, newStatus); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	h.Hub.BroadcastEvent("plan_updated", map[string]interface{}{"plan_id": planID, "status": newStatus})
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "new_status": newStatus})
}
