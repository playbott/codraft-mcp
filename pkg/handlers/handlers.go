package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"codraft-mcp/pkg/db"
	"codraft-mcp/pkg/ws"
)

type Handler struct {
	DBM *db.DBManager
	Hub *ws.Hub
}

func New(dbm *db.DBManager, hub *ws.Hub) *Handler {
	return &Handler{DBM: dbm, Hub: hub}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/tasks", h.HandleTasks)
	mux.HandleFunc("/api/tasks/", h.HandleTaskByID)
	mux.HandleFunc("/api/ping", h.HandlePing)
	mux.HandleFunc("/api/plans", h.HandlePlans)
	mux.HandleFunc("/api/plans/", h.HandlePlanByID)
	mux.HandleFunc("/api/comments", h.HandleComments)
	mux.HandleFunc("/api/issues/", h.HandleIssueByID)
	mux.HandleFunc("/api/walkthroughs/", h.HandleWalkthroughs)

	mux.HandleFunc("/api/folders/rename", h.HandleFolderRename)
	mux.HandleFunc("/api/folders/delete", h.HandleFolderDelete)
	mux.HandleFunc("/api/folders", h.HandleFolders)
	mux.HandleFunc("/api/settings", h.HandleSettings)
	mux.HandleFunc("/api/projects", h.HandleProjects)
	mux.HandleFunc("/api/config/loglevel", h.HandleLogLevel)
	mux.HandleFunc("/ws", h.Hub.HandleWS)
}

func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

type responseLogger struct {
	http.ResponseWriter
	status int
}

func (r *responseLogger) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseLogger) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := r.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("http.Hijacker not supported")
}

func (r *responseLogger) Flush() {
	if fl, ok := r.ResponseWriter.(http.Flusher); ok {
		fl.Flush()
	}
}

func LoggingMiddleware(logFn func(format string, args ...interface{}), next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rl := &responseLogger{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rl, r)
		duration := time.Since(start)

		if r.URL.Path == "/api/ping" {
			return
		}
		if rl.status >= 400 || r.Method != http.MethodGet {
			logFn("%s %s %d %s (%v)", r.Method, r.URL.RequestURI(), rl.status, r.RemoteAddr, duration)
		}
	})
}
