package handlers

import (
	"encoding/json"
	"net/http"

	"codraft-mcp/pkg/db"
)

func (h *Handler) HandleComments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		entityType := r.URL.Query().Get("entity_type")
		entityID := r.URL.Query().Get("entity_id")
		if entityType == "" || entityID == "" {
			http.Error(w, "entity_type and entity_id query params required", http.StatusBadRequest)
			return
		}
		var db *db.DB
		if entityType == "plan" {
			if d, _, err := h.DBM.FindPlanDB(entityID); err == nil {
				db = d
			}
		} else if entityType == "task" {
			if d, _, err := h.DBM.FindTaskDB(entityID); err == nil {
				db = d
			}
		}
		if db == nil {
			db = h.DBM.GetDB("")
		}
		comments, err := db.GetComments(entityType, entityID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(comments)

	case http.MethodPost:
		var body struct {
			EntityType string `json:"entity_type"`
			EntityID   string `json:"entity_id"`
			Text       string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.EntityType == "" || body.EntityID == "" || body.Text == "" {
			http.Error(w, "entity_type, entity_id and text are required", http.StatusBadRequest)
			return
		}

		var db *db.DB
		if body.EntityType == "plan" {
			if d, _, err := h.DBM.FindPlanDB(body.EntityID); err == nil {
				db = d
			}
		} else if body.EntityType == "task" {
			if d, _, err := h.DBM.FindTaskDB(body.EntityID); err == nil {
				db = d
			}
		}
		if db == nil {
			db = h.DBM.GetDB("")
		}

		id, err := db.AddComment(body.EntityType, body.EntityID, "user", body.Text)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		h.Hub.BroadcastEvent("comment_added", map[string]interface{}{
			"comment_id":  id,
			"entity_type": body.EntityType,
			"entity_id":   body.EntityID,
		})
		WriteJSON(w, http.StatusCreated, map[string]string{"id": id})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query param required", http.StatusBadRequest)
			return
		}
		delDB := h.DBM.GetDB("")
		if err := delDB.DeleteComment(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		h.Hub.BroadcastEvent("comment_deleted", map[string]interface{}{"comment_id": id})
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}
