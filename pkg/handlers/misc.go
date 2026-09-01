package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"
)

var (
	AppName    = "codraft-mcp"
	AppVersion = "dev"

	fnMu            sync.RWMutex
	RegistryTouchFn func()
	LogLevelGetFn   func() string
	LogLevelSetFn   func(string) string
)

func (h *Handler) HandlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	fnMu.RLock()
	touch := RegistryTouchFn
	fnMu.RUnlock()
	if touch != nil {
		go touch()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"app":     AppName,
		"version": AppVersion,
		"message": "Pong! Go backend is running smoothly.",
		"time":    time.Now().Format("15:04:05"),
	})
}

func (h *Handler) HandleFolderRename(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		OldName string `json:"old_name"`
		NewName string `json:"new_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.OldName == "" {
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	db := h.DBM.GetDB("")
	if err := db.RenameFolder(body.OldName, body.NewName); err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.Hub.BroadcastEvent("folder_renamed", map[string]interface{}{"old_name": body.OldName, "new_name": body.NewName})
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HandleFolderDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" && r.Body != nil {
		var body struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		name = body.Name
	}
	if name == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	db := h.DBM.GetDB("")
	if err := db.DeleteFolder(name); err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	h.Hub.BroadcastEvent("folder_deleted", map[string]interface{}{"name": name})
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HandleFolders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		db := h.DBM.GetDB("")
		folders, err := db.GetFolders()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(folders)

	case http.MethodDelete:
		h.HandleFolderDelete(w, r)

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) HandleSettings(w http.ResponseWriter, r *http.Request) {
	db := h.DBM.GetDB("")
	switch r.Method {
	case http.MethodGet:
		key := r.URL.Query().Get("key")
		if key != "" {
			val, err := db.GetSetting(key)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			WriteJSON(w, http.StatusOK, map[string]string{"key": key, "value": val})
			return
		}
		settings, err := db.GetSettings()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(settings)

	case http.MethodPost:
		var body struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.Key == "" {
			http.Error(w, "key is required", http.StatusBadRequest)
			return
		}
		if err := db.SetSetting(body.Key, body.Value); err != nil {
			WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		h.Hub.BroadcastEvent("setting_updated", map[string]interface{}{"key": body.Key, "value": body.Value})
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "key": body.Key, "value": body.Value})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) HandleProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	projects, err := h.DBM.GetDefaultDB().GetProjects()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projects)
}

type SessionsInfo struct {
	CurrentProject string
	ProjectPath    string
	Port           int
}

func (h *Handler) HandleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	sessions, err := h.DBM.GetDefaultDB().GetSessions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var sessionProjects []string
	for _, s := range sessions {
		sessionProjects = append(sessionProjects, s.Project)
	}

	portStr := r.Header.Get("X-Server-Port")
	port, _ := strconv.Atoi(portStr)
	currentProject := r.Header.Get("X-Current-Project")
	projectPath := r.Header.Get("X-Project-Path")

	result := map[string]interface{}{
		"port":            port,
		"current_project": currentProject,
		"project_path":    projectPath,
		"projects":        sessionProjects,
		"sessions":        sessions,
	}
	WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) HandleLogLevel(w http.ResponseWriter, r *http.Request) {
	fnMu.RLock()
	getFn := LogLevelGetFn
	setFn := LogLevelSetFn
	fnMu.RUnlock()

	switch r.Method {
	case http.MethodGet:
		level := "INFO"
		if getFn != nil {
			level = getFn()
		}
		WriteJSON(w, http.StatusOK, map[string]string{"level": level})
	case http.MethodPost:
		var body struct {
			Level string `json:"level"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		level := body.Level
		if setFn != nil {
			level = setFn(body.Level)
		}
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "level": level})
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}
