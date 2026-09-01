package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	appdb "codraft-mcp/pkg/db"
	"codraft-mcp/pkg/handlers"
	appws "codraft-mcp/pkg/ws"
)

func TestRouteRegistration(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("HTTP ServeMux route registration panicked: %v", r)
		}
	}()

	tmpDir := t.TempDir()
	dbm, err := appdb.NewDBManager(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("NewDBManager failed: %v", err)
	}
	defer dbm.Close()

	hub := appws.NewHub()
	h := handlers.New(dbm, hub)
	mux := http.NewServeMux()

	mux.HandleFunc("/api/sessions", func(w http.ResponseWriter, r *http.Request) {
		h.HandleSessions(w, r)
	})
	h.Register(mux)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {})
}

func TestFileLogger(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := tmpDir + "/codraft.log"
	InitFileLogger(logPath)
	defer CloseFileLogger()
	LogInfo("TEST", "File logger test message")
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("Log file was not created: %v", err)
	}
}

func TestStaticAndAPIServing(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test_tracker.db"

	dbm, err := appdb.NewDBManager(dbPath)
	if err != nil {
		t.Fatalf("NewDBManager failed: %v", err)
	}
	defer dbm.Close()

	hub := appws.NewHub()
	h := handlers.New(dbm, hub)
	mux := http.NewServeMux()
	h.Register(mux)

	uiSub, err := fsSubUI()
	if err != nil {
		t.Fatalf("fsSubUI failed: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(uiSub)))

	ts := httptest.NewServer(mux)
	defer ts.Close()

	endpoints := []string{
		"/",
		"/js/main.js",
		"/js/i18n.js",
		"/js/state.js",
		"/locales/ru.json",
		"/api/ping",
		"/api/plans",
	}

	for _, ep := range endpoints {
		resp, err := http.Get(ts.URL + ep)
		if err != nil {
			t.Fatalf("GET %s failed: %v", ep, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s returned status %d; body: %s", ep, resp.StatusCode, string(body))
		}
	}
}

func TestSettingsStorage(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test_tracker.db"

	db, err := appdb.NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	testKeys := map[string]string{
		"selectedfolder": "Frontend-Refactor",
		"lastopenedplan": "plan-1712345678",
		"lang":           "ru",
		"loglevel":       "DEBUG",
	}

	for k, v := range testKeys {
		if err := db.SetSetting(k, v); err != nil {
			t.Fatalf("SetSetting(%s, %s) failed: %v", k, v, err)
		}
		got, err := db.GetSetting(k)
		if err != nil {
			t.Fatalf("GetSetting(%s) failed: %v", k, err)
		}
		if got != v {
			t.Fatalf("GetSetting(%s) = %s; want %s", k, got, v)
		}
	}

	settings, err := db.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings() failed: %v", err)
	}
	if len(settings) < len(testKeys) {
		t.Fatalf("GetSettings() returned %d items; want at least %d", len(settings), len(testKeys))
	}
}

func TestPlanReturnToDraft(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test_tracker.db"

	db, err := appdb.NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	planID, err := db.CreatePlan("Test Plan", "Desc", "test-project", "")
	if err != nil {
		t.Fatalf("CreatePlan failed: %v", err)
	}

	if err := db.UpdatePlanStatus(planID, "rejected"); err != nil {
		t.Fatalf("UpdatePlanStatus(rejected) failed: %v", err)
	}

	if err := db.UpdatePlanStatus(planID, "draft"); err != nil {
		t.Fatalf("UpdatePlanStatus(draft from rejected) failed: %v", err)
	}

	if err := db.UpdatePlanStatus(planID, "approved"); err != nil {
		t.Fatalf("UpdatePlanStatus(approved) failed: %v", err)
	}
	if err := db.UpdatePlanStatus(planID, "canceled"); err != nil {
		t.Fatalf("UpdatePlanStatus(canceled) failed: %v", err)
	}

	if err := db.UpdatePlanStatus(planID, "draft"); err != nil {
		t.Fatalf("UpdatePlanStatus(draft from canceled) failed: %v", err)
	}
}
