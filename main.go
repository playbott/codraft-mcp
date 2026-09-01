package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	appdb "codraft-mcp/pkg/db"
	"codraft-mcp/pkg/handlers"
	appws "codraft-mcp/pkg/ws"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

//go:embed ui
var uiFS embed.FS

var (
	primaryMu       sync.Mutex
	primaryServer   bool
	httpListenerVar net.Listener

	clientInfoAtomic   atomic.Value
	ownRegistryKeyAtom atomic.Value

	handshakeDoneCh   = make(chan struct{})
	handshakeDoneOnce sync.Once
)

func setClientInfo(ci mcp.Implementation) {
	clientInfoAtomic.Store(ci)
}

func currentClientInfo() (mcp.Implementation, bool) {
	v := clientInfoAtomic.Load()
	if v == nil {
		return mcp.Implementation{}, false
	}
	return v.(mcp.Implementation), true
}

func setPrimaryServer(v bool) {
	primaryMu.Lock()
	defer primaryMu.Unlock()
	primaryServer = v
}

func primaryIsServer() bool {
	primaryMu.Lock()
	defer primaryMu.Unlock()
	return primaryServer
}

func setHTTPListener(l net.Listener) {
	primaryMu.Lock()
	defer primaryMu.Unlock()
	httpListenerVar = l
}

func setOwnRegistryKey(k string) {
	ownRegistryKeyAtom.Store(k)
}

func registryOwnKey() string {
	v := ownRegistryKeyAtom.Load()
	if v == nil {
		return ""
	}
	return v.(string)
}

func closeHandshake() {
	handshakeDoneOnce.Do(func() {
		close(handshakeDoneCh)
	})
}

func registryCurrentEntry() RegistryEntry {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	ci, known := currentClientInfo()
	client := RegistryClientPending
	version := ""
	if known && strings.TrimSpace(ci.Name) != "" {
		client = strings.TrimSpace(ci.Name)
		version = ci.Version
	}
	return RegistryEntry{
		Port:          getKnownPort(),
		PID:           os.Getpid(),
		Project:       autoProject(),
		Cwd:           cwd,
		Client:        client,
		ClientVersion: version,
		IDE:           clientToIDE(client),
	}
}

func uiURL(port int) string {
	u := fmt.Sprintf("http://localhost:%d", port)
	ci, known := currentClientInfo()
	client := RegistryClientPending
	ide := "unknown"
	if known && strings.TrimSpace(ci.Name) != "" {
		client = strings.TrimSpace(ci.Name)
		ide = clientToIDE(client)
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	u += "?client=" + url.QueryEscape(client) +
		"&ide=" + url.QueryEscape(ide) +
		"&cwd=" + url.QueryEscape(cwd) +
		"&pid=" + strconv.Itoa(os.Getpid())
	return u
}

func serverAlive(port int) bool {
	if port <= 0 {
		return false
	}
	client := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/ping", port))
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func handleClientHandshake() {
	if !primaryIsServer() {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	ci, known := currentClientInfo()
	client := RegistryClientPending
	if known && strings.TrimSpace(ci.Name) != "" {
		client = strings.TrimSpace(ci.Name)
	}

	if err := registryRemoveKeyIfPid(registryKey(RegistryClientPending, cwd), os.Getpid()); err != nil {
		LogWarn("REG", "Failed to remove pending registry entry: %v", err)
	}

	entry := registryCurrentEntry()
	key := registryKey(entry.Client, entry.Cwd)
	ourPort := getKnownPort()

	claimIfStale := func(existing RegistryEntry) bool {
		if existing.PID == os.Getpid() || existing.Port == ourPort {
			return false
		}
		if serverAlive(existing.Port) {
			LogWarn("REG", "Duplicate server for client '%s' (existing pid %d on port %d IS ALIVE); switching to client mode.", client, existing.PID, existing.Port)
			switchToClientMode(existing.Port)
			return true
		}
		LogWarn("REG", "Registry entry for client '%s' is STALE (pid %d port %d unreachable); claiming primary.", client, existing.PID, existing.Port)
		return false
	}

	if client != RegistryClientPending {
		if existing, ok := registryFindLive(key); ok {
			if claimIfStale(existing) {
				return
			}
		}
	}

	if err := registryUpsert(entry); err != nil {
		LogError("REG", "Failed to upsert registry entry after handshake: %v", err)
	}
	setOwnRegistryKey(key)
	LogInfo("REG", "Client identified via handshake: name=%s version=%s ide=%s (key=%s port=%d pid=%d)", entry.Client, entry.ClientVersion, entry.IDE, key, ourPort, os.Getpid())

	if existing, ok := registryFindLive(key); ok {
		if claimIfStale(existing) {
			return
		}
	}
	closeHandshake()
}

func switchToClientMode(existingPort int) {
	primaryMu.Lock()
	if !primaryServer {
		primaryMu.Unlock()
		return
	}
	primaryServer = false
	listener := httpListenerVar
	primaryMu.Unlock()

	LogWarn("MAIN", "Duplicate detected; switching to client mode, reusing port %d", existingPort)
	if listener != nil {
		_ = listener.Close()
	}
	_ = registryRemoveByPid(os.Getpid())
	setOwnRegistryKey("")
	setKnownPort(existingPort)
	writePortFile(existingPort)
	closeHandshake()
}

func registryTouchOwn() {
	if !primaryIsServer() {
		return
	}
	key := registryOwnKey()
	if key == "" {
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		key = registryKey(RegistryClientPending, cwd)
	}
	_ = registryTouch(key)
}

func registryHeartbeat() {
	if !primaryIsServer() {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	key := registryOwnKey()
	if key == "" {
		key = registryKey(RegistryClientPending, cwd)
	}

	if existing, ok := registryFindLive(key); ok && existing.PID != os.Getpid() && existing.Port != getKnownPort() {
		if serverAlive(existing.Port) {
			LogWarn("REG", "Heartbeat: entry %s owned by LIVE pid %d (port %d); switching to client mode.", key, existing.PID, existing.Port)
			switchToClientMode(existing.Port)
			return
		}
		LogWarn("REG", "Heartbeat: entry %s is STALE (pid %d port %d unreachable); re-claiming primary.", key, existing.PID, existing.Port)
		_ = registryUpsert(registryCurrentEntry())
		return
	}

	if existing, ok := registryFindLive(key); !ok || existing.PID != os.Getpid() {
		_ = registryUpsert(registryCurrentEntry())
	} else {
		_ = registryTouch(key)
	}
}

func registryHeartbeatLoop() {
	ticker := time.NewTicker(RegistryHeartbeatInterval)
	defer ticker.Stop()
	for range ticker.C {
		_ = registryCleanupStale()
		registryHeartbeat()
	}
}

func shutdownCleanup() {
	if err := registryRemoveByPid(os.Getpid()); err != nil {
		LogWarn("REG", "Cleanup: failed to remove registry entries for pid %d: %v", os.Getpid(), err)
	}
	cleanupPortFile()
}

func main() {
	dbPath, _ := filepath.Abs(getDBPath())
	logPath := filepath.Join(filepath.Dir(dbPath), "codraft.log")
	InitFileLogger(logPath)
	defer CloseFileLogger()

	autoRegisterIDE()

	LogInfo("MAIN", "%s %s initializing (Log Level: %s)...", AppDisplayName, AppVersion, GetLogLevel().String())

	dbm, err := appdb.NewDBManager(dbPath)
	if err != nil {
		LogError("MAIN", "Database initialization error: %v", err)
		log.Fatalf("Database error: %v\n", err)
	}
	defer dbm.Close()

	if proj := autoProject(); proj != "" {
		dbm.SetActiveProject(proj)
		dbm.GetDefaultDB().UpsertSession(proj)
	}

	hub := appws.NewHub()

	cwd, _ := os.Getwd()

	if existingPort, isRunning := checkExistingServer(); isRunning {
		if registryHasLiveEntryForCwd(cwd) {
			LogInfo("MAIN", "Existing %s server for project '%s' on port %d is registry-managed; starting own instance (duplicate resolution after MCP handshake).", AppDisplayName, autoProject(), existingPort)
		} else {
			LogInfo("MAIN", "Existing legacy %s server for project '%s' detected running on port %d. Reusing existing HTTP server instance.", AppDisplayName, autoProject(), existingPort)
			setKnownPort(existingPort)
			serveStdioAndExit(dbm, hub)
		}
	}

	var lock *ProjectLock
	if l, acquired := tryAcquireProjectLock(); acquired {
		lock = l
	} else {
		if existingPort, isRunning := waitForExistingServer(3 * time.Second); isRunning {
			if registryHasLiveEntryForCwd(cwd) {
				LogInfo("MAIN", "Sibling registry-managed server on port %d; starting own instance.", existingPort)
			} else {
				LogInfo("MAIN", "Sibling legacy server finished starting on port %d. Reusing existing HTTP server instance.", existingPort)
				setKnownPort(existingPort)
				serveStdioAndExit(dbm, hub)
			}
		} else {
			LogWarn("MAIN", "File lock held by an unresponsive process. Sending handover stop signal...")
			sendHandoverSignal()
			for i := 0; i < 15; i++ {
				time.Sleep(100 * time.Millisecond)
				if l, ok := tryAcquireProjectLock(); ok {
					lock = l
					LogInfo("MAIN", "Acquired project file lock after handover.")
					break
				}
			}
			if lock == nil {
				if existingPort, isRunning := checkExistingServer(); isRunning {
					if registryHasLiveEntryForCwd(cwd) {
						LogInfo("MAIN", "Handover: registry-managed server answered on port %d; starting own instance.", existingPort)
					} else {
						LogWarn("MAIN", "Handover failed but a legacy server answered on port %d. Proceeding in client mode.", existingPort)
						setKnownPort(existingPort)
						serveStdioAndExit(dbm, hub)
					}
				} else {
					LogWarn("MAIN", "Could not acquire file lock after handover wait and no server responded. Starting as primary anyway.")
				}
			}
		}
	}
	if lock != nil {
		go func() {
			select {
			case <-handshakeDoneCh:
			case <-time.After(15 * time.Second):
			}
			lock.Release()
		}()
	}

	watchHandoverSignal(func() {
		shutdownCleanup()
	})

	go hub.Run()

	mux := http.NewServeMux()

	handlers.LogLevelGetFn = func() string { return GetLogLevel().String() }
	handlers.LogLevelSetFn = func(s string) string { return SetLogLevelString(s).String() }
	handlers.AppName = AppName
	handlers.AppVersion = AppVersion
	handlers.RegistryTouchFn = registryTouchOwn

	h := handlers.New(dbm, hub)

	mux.HandleFunc("/api/sessions", func(w http.ResponseWriter, r *http.Request) {
		portStr := readPortFile()
		r.Header.Set("X-Server-Port", portStr)
		r.Header.Set("X-Current-Project", autoProject())
		cwd, _ := os.Getwd()
		r.Header.Set("X-Project-Path", cwd)
		h.HandleSessions(w, r)
	})

	h.Register(mux)

	uiSub, err := fsSubUI()
	if err != nil {
		log.Fatalf("Failed to load embedded UI: %v\n", err)
	}
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" && r.URL.Query().Get("client") == "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><title>Codraft MCP</title></head>
<body style="font-family:sans-serif;margin:40px;color:#222">
<h2>CoDraft MCP</h2>
<p style="background:#c62828;color:#fff;padding:12px;border-radius:4px">
This tab was opened <b>without client parameters</b> (client/ide/cwd/pid). Please close it.
</p>
<p>CoDraft UI opens automatically in your IDE's Simple Browser with client parameters.</p>
</body></html>`)
			return
		}
		http.FileServer(http.FS(uiSub)).ServeHTTP(w, r)
	}))

	_ = registryCleanupStale()

	go func() {
		listener, err := net.Listen("tcp", ":"+getPort())
		if err != nil {
			LogError("HTTP", "Failed to start HTTP server: %v", err)
			log.Fatalf("Failed to start HTTP server: %v\n", err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		setKnownPort(port)
		setPrimaryServer(true)
		setHTTPListener(listener)
		writePortFile(port)
		entry := registryCurrentEntry()
		registryUpsert(entry)
		if entry.Client == RegistryClientPending {
			if ci, known := currentClientInfo(); known && strings.TrimSpace(ci.Name) != "" {
				LogInfo("REG", "Pending registration skipped (handshake already completed: %s)", ci.Name)
			} else {
				setOwnRegistryKey(registryKey(entry.Client, entry.Cwd))
			}
		} else {
			setOwnRegistryKey(registryKey(entry.Client, entry.Cwd))
		}
		LogInfo("REG", "Registered in registry: key=%s port=%d pid=%d", registryKey(entry.Client, entry.Cwd), port, os.Getpid())
		printBanner(port)
		LogInfo("HTTP", "HTTP server listening on port %d", port)
		logFn := func(format string, args ...interface{}) {
			LogInfo("HTTP", format, args...)
		}
		if err := http.Serve(listener, handlers.LoggingMiddleware(logFn, mux)); err != nil {
			if primaryIsServer() {
				LogError("HTTP", "HTTP server error: %v", err)
				log.Fatalf("HTTP server error: %v\n", err)
			}
			LogInfo("HTTP", "HTTP listener closed after switch to client mode.")
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		LogWarn("MAIN", "Received shutdown signal, stopping server...")
		shutdownCleanup()
		os.Exit(0)
	}()

	go registryHeartbeatLoop()

	mcpServer := NewMCPServer(dbm, hub)
	LogInfo("MCP", "Starting MCP STDIO server...")
	mcpDone := make(chan struct{})
	go func() {
		defer close(mcpDone)
		if err := server.ServeStdio(mcpServer); err != nil {
			LogError("MCP", "MCP server error: %v", err)
			log.Printf("MCP server error: %v\n", err)
		}
	}()
	<-mcpDone
	shutdownCleanup()
	LogInfo("MAIN", "MCP STDIO session finished, exiting process...")
	os.Exit(0)
}

func serveStdioAndExit(dbm *appdb.DBManager, hub *appws.Hub) {
	mcpServer := NewMCPServer(dbm, hub)
	LogInfo("MCP", "Starting MCP STDIO server for client connection...")
	if err := server.ServeStdio(mcpServer); err != nil {
		LogError("MCP", "MCP server error: %v", err)
	}
	LogInfo("MAIN", "MCP STDIO client session ended, exiting process...")
	dbm.Close()
	os.Exit(0)
}

func portToStr(port int) string {
	return strconv.Itoa(port)
}

func fsSubUI() (fs.FS, error) {
	return fs.Sub(uiFS, "ui")
}
