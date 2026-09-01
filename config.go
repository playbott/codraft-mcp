package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	appdb "codraft-mcp/pkg/db"
)

const (
	AppName        = "codraft-mcp"
	AppDisplayName = "CoDraft MCP"
	AppShortName   = "CoDraft"

	AppVersion = "0.4.0-alpha"

	HelperExtVersion   = "1.8.0"
	HelperExtPublisher = "codraft"

	HelperExtName  = "codraft-ui-opener"
	HelperExtDir   = "codraft.codraft-ui-opener"
	HelperExtTitle = "CoDraft UI Opener"

	DefaultPortFileName = "tracker.port"
)

var (
	legacyAppNames = []string{"task-tracker"}
	legacyExtNames = []string{"task-tracker-ui-opener", "codraft-ui-opener"}
)

func getDBPath() string {
	if p := os.Getenv("TRACKER_DB_PATH"); p != "" {
		return p
	}
	cwd, err := os.Getwd()
	if err == nil && cwd != "" {
		return appdb.EnsureCodraftDirAndMigrate(cwd)
	}
	exe, err := os.Executable()
	if err != nil {
		return filepath.Join(appdb.DefaultCodraftDir, appdb.DefaultDBFileName)
	}
	return appdb.EnsureCodraftDirAndMigrate(filepath.Dir(exe))
}

func getPort() string {
	if p := os.Getenv("TRACKER_PORT"); p != "" {
		return p
	}
	return "0"
}

var knownPort atomic.Int32

func setKnownPort(port int) {
	knownPort.Store(int32(port))
}

func getKnownPort() int {
	if p := knownPort.Load(); p > 0 {
		return int(p)
	}
	portStr := strings.TrimSpace(readPortFile())
	p, err := strconv.Atoi(portStr)
	if err != nil || p <= 0 {
		return 0
	}
	return p
}

func writePortFile(port int) {
	pPath := portFile()
	if err := os.WriteFile(pPath, []byte(strconv.Itoa(port)), 0644); err != nil {
		LogError("CONFIG", "Failed to write port file %s: %v", pPath, err)
	}
}

func readPortFile() string {
	if data, err := os.ReadFile(portFile()); err == nil && len(data) > 0 {
		return string(data)
	}
	cwd, err := os.Getwd()
	if err == nil {
		vscPort := filepath.Join(cwd, ".vscode", DefaultPortFileName)
		if data, err := os.ReadFile(vscPort); err == nil && len(data) > 0 {
			return string(data)
		}
		rootPort := filepath.Join(cwd, DefaultPortFileName)
		if data, err := os.ReadFile(rootPort); err == nil && len(data) > 0 {
			return string(data)
		}
	}
	return ""
}

func checkExistingServer() (int, bool) {
	portStr := getPort()
	if portStr == "0" || portStr == "" {
		portStr = readPortFile()
	}
	port, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil || port <= 0 {
		return 0, false
	}

	client := &http.Client{
		Timeout: 250 * time.Millisecond,
	}

	url := fmt.Sprintf("http://localhost:%d/api/ping", port)
	resp, err := client.Get(url)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, false
	}

	var data struct {
		Status  string `json:"status"`
		App     string `json:"app"`
		Project string `json:"project"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, false
	}

	if data.Status == "ok" && (data.App == AppName || data.App == "task-tracker-mcp" || data.App == "codraft-mcp") {
		return port, true
	}

	return 0, false
}

func waitForExistingServer(timeout time.Duration) (int, bool) {
	deadline := time.Now().Add(timeout)
	for {
		if port, ok := checkExistingServer(); ok {
			return port, true
		}
		if time.Now().After(deadline) {
			return 0, false
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func writeFileIfChanged(path string, data []byte) bool {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return false
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		LogError("CONFIG", "Failed to write %s: %v", path, err)
		return false
	}
	return true
}

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func portFile() string {
	cwd, err := os.Getwd()
	if err != nil {
		return filepath.Join(appdb.DefaultCodraftDir, DefaultPortFileName)
	}
	dir := filepath.Join(cwd, appdb.DefaultCodraftDir)
	_ = os.MkdirAll(dir, 0755)
	return filepath.Join(dir, DefaultPortFileName)
}

func cleanupPortFile() {
	if !primaryIsServer() {
		return
	}
	pPath := portFile()
	if data, err := os.ReadFile(pPath); err == nil {
		port, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		kp := int(knownPort.Load())
		if kp > 0 && port != kp {
			return
		}
	}
	_ = os.Remove(pPath)
}

func shouldOpenBrowser() bool {
	return os.Getenv("TRACKER_OPEN_BROWSER") == "1"
}

func autoProject() string {
	if p := os.Getenv("TRACKER_PROJECT"); p != "" {
		return p
	}
	cwd, err := os.Getwd()
	if err == nil && cwd != "" {
		return filepath.Base(cwd)
	}
	return "project"
}

func openBrowser(port int) {
	url := uiURL(port)
	LogWarn("CONFIG", "Opening external browser (port %d)", port)
	_ = openBrowserPlatform(url)
}

func printBanner(port int) {
	banner := fmt.Sprintf(`
================ %s %s ================
  DB:      %s
  Project: %s
  Status:  ALPHA - Windows only, IDE support: COMPATIBILITY.md
=========================================================
`, AppDisplayName, AppVersion, getDBPath(), autoProject())
	_, _ = fmt.Fprint(os.Stderr, banner)
}

var extPackageJSON = fmt.Sprintf(`{
  "name": "%s",
  "displayName": "%s",
  "publisher": "%s",
  "description": "Automatically opens the Simple Browser when %s starts.",
  "version": "%s",
  "engines": {
    "vscode": "^1.75.0"
  },
  "categories": [
    "Other"
  ],
  "activationEvents": [
    "*",
    "onStartupFinished",
    "workspaceContains:%s/%s",
    "workspaceContains:.vscode/%s",
    "workspaceContains:%s"
  ],
  "main": "./extension.js",
  "contributes": {}
}`, HelperExtName, HelperExtTitle, HelperExtPublisher, AppDisplayName, HelperExtVersion, appdb.DefaultCodraftDir, DefaultPortFileName, DefaultPortFileName, DefaultPortFileName)

var extExtensionJS = fmt.Sprintf(`
const vscode = require('vscode');
const fs = require('fs');
const path = require('path');
const http = require('http');
const os = require('os');

const PORT_FILE_NAME = '%s';
const CODRAFT_DIR = '%s';
const REGISTRY_FILE_NAME = 'ports.json';
const REGISTRY_TTL_MS = 45000;

let watcher = null;
let lastOpenedPort = null;
let debounceTimer = null;
let healthTimer = null;
let sweepInProgress = false;

function extLog(msg) {
    try {
        const p = path.join(os.tmpdir(), 'codraft-extension.log');
        fs.appendFileSync(p, new Date().toISOString() + ' ' + msg + '\n');
    } catch (e) {}
}

function normalizePath(p) {
    if (!p) return '';
    return String(p).toLowerCase().replace(/\\/g, '/').replace(/\/+$/, '');
}

function readPortFromFile(portFilePath) {
    try {
        if (!fs.existsSync(portFilePath)) return 0;
        const content = fs.readFileSync(portFilePath, 'utf8').trim();
        if (!/^\d+$/.test(content)) return 0;
        return parseInt(content, 10);
    } catch (e) {
        return 0;
    }
}

async function closeSimpleBrowserTabs() {
    try {
        const tabGroups = vscode.window.tabGroups;
        if (!tabGroups || !tabGroups.all) return;

        const tabsToClose = [];
        for (const group of tabGroups.all) {
            for (const tab of group.tabs) {
                if (tab.input && tab.input.viewType === 'simpleBrowser.view') {
                    tabsToClose.push(tab);
                    continue;
                }
                if (tab.label) {
                    const label = tab.label.toLowerCase();
                    if (label.includes('simple browser') || label.includes('task tracker') || label.includes('tracker') || label.includes('codraft')) {
                        tabsToClose.push(tab);
                    }
                }
            }
        }
        if (tabsToClose.length > 0) {
            await tabGroups.close(tabsToClose, true);
        }
    } catch (e) {
        console.error('Error closing Simple Browser tabs', e);
    }
}

function checkServerHealth(port) {
    if (!port) return;
    const req = http.get("http://localhost:" + port + "/api/ping", { timeout: 1000 }, (res) => {
        if (res.statusCode !== 200) {
            onServerDead();
        }
    });
    req.on('error', () => {
        onServerDead();
    });
}

function onServerDead() {
    if (lastOpenedPort) {
        extLog('server dead, closing tab (port=' + lastOpenedPort + ')');
        lastOpenedPort = null;
        closeSimpleBrowserTabs();
    }
}

function registryUrlForPort(port) {
    try {
        const regPath = path.join(os.homedir(), CODRAFT_DIR, REGISTRY_FILE_NAME);
        if (!fs.existsSync(regPath)) return "http://localhost:" + port;
        const data = JSON.parse(fs.readFileSync(regPath, 'utf8'));
        if (!data || typeof data !== 'object') return "http://localhost:" + port;
        for (const key in data) {
            const entry = data[key];
            if (entry && typeof entry === 'object' && parseInt(entry.port, 10) === port) {
                let url = "http://localhost:" + port;
                if (entry.client || entry.ide || entry.cwd || entry.pid) {
                    url += "?client=" + encodeURIComponent(entry.client || '') +
                        "&ide=" + encodeURIComponent(entry.ide || '') +
                        "&cwd=" + encodeURIComponent(entry.cwd || '') +
                        "&pid=" + encodeURIComponent(entry.pid || '');
                }
                return url;
            }
        }
    } catch (e) {}
    return "http://localhost:" + port;
}

function queueOpenSimpleBrowser(port, url) {
    if (!port || !url) return;

    if (port === lastOpenedPort) {
        return;
    }

    if (debounceTimer) {
        clearTimeout(debounceTimer);
    }

    debounceTimer = setTimeout(async () => {
        debounceTimer = null;
        try {
            extLog('show tab: ' + url);
            await closeSimpleBrowserTabs();
            lastOpenedPort = port;
            await vscode.commands.executeCommand('simpleBrowser.show', url);
        } catch (e) {
            extLog('error opening Simple Browser: ' + e);
        }
    }, 400);
}

function sweepStaleTabs(rootPath, portFiles) {
    if (sweepInProgress) return;
    const count = countSimpleBrowserTabs();
    if (count <= 1) {
        extLog('sweep: tabs=' + count + ', nothing to close');
        return;
    }
    extLog('sweep: tabs=' + count + ', closing extras and reopening ours');
    sweepInProgress = true;
    closeSimpleBrowserTabs().then(() => {
        sweepInProgress = false;
        lastOpenedPort = null;
        for (const f of portFiles) {
            const port = readPortFromFile(f);
            if (port > 0) {
                queueOpenSimpleBrowser(port, registryUrlForPort(port));
                return;
            }
        }
    });
}

function countSimpleBrowserTabs() {
    try {
        const tabGroups = vscode.window.tabGroups;
        if (!tabGroups || !tabGroups.all) return 0;
        let count = 0;
        for (const group of tabGroups.all) {
            for (const tab of group.tabs) {
                if (tab.input && tab.input.viewType === 'simpleBrowser.view') {
                    count++;
                } else if (tab.label) {
                    const l = tab.label.toLowerCase();
                    if (l.includes('simple browser') || l.includes('codraft') || l.includes('tracker') || l.includes('localhost')) {
                        count++;
                    }
                }
            }
        }
        return count;
    } catch (e) {
        return 0;
    }
}

function checkInitial(rootPath, portFiles) {
    for (const f of portFiles) {
        const port = readPortFromFile(f);
        if (port <= 0) continue;
        const req = http.get("http://127.0.0.1:" + port + "/api/ping", { timeout: 800 }, (res) => {
            if (res.statusCode === 200) {
                console.log('[codraft-ui-opener] initial check: server alive on ' + port + ', opening tab');
                queueOpenSimpleBrowser(port, registryUrlForPort(port));
            } else {
                console.log('[codraft-ui-opener] initial check: port ' + port + ' not responding, waiting for watcher event');
            }
        });
        req.on('error', () => {
            console.log('[codraft-ui-opener] initial check: port ' + port + ' unreachable, waiting for watcher event');
        });
        req.setTimeout(800, () => { req.destroy(); });
        return;
    }
}

function activate(context) {
    const workspaceFolders = vscode.workspace.workspaceFolders;
    if (!workspaceFolders) return;

    const rootPath = workspaceFolders[0].uri.fsPath;
    const portFiles = [
        path.join(rootPath, CODRAFT_DIR, PORT_FILE_NAME),
        path.join(rootPath, '.vscode', PORT_FILE_NAME),
        path.join(rootPath, PORT_FILE_NAME)
    ];

    setTimeout(() => {
        checkInitial(rootPath, portFiles);
    }, 1000);

    watcher = vscode.workspace.createFileSystemWatcher('{**/' + CODRAFT_DIR + '/' + PORT_FILE_NAME + ',**/.vscode/' + PORT_FILE_NAME + ',**/' + PORT_FILE_NAME + '}');

    watcher.onDidCreate((uri) => {
        const port = readPortFromFile(uri.fsPath);
        if (port > 0) {
            queueOpenSimpleBrowser(port, registryUrlForPort(port));
        }
    });
    watcher.onDidChange((uri) => {
        const port = readPortFromFile(uri.fsPath);
        if (port > 0) {
            queueOpenSimpleBrowser(port, registryUrlForPort(port));
        }
    });
    watcher.onDidDelete(() => {
    });

    healthTimer = setInterval(() => {
        if (lastOpenedPort) {
            checkServerHealth(lastOpenedPort);
        }
    }, 2000);

    setTimeout(() => {
        sweepStaleTabs(rootPath, portFiles);
    }, 3000);
    setTimeout(() => {
        sweepStaleTabs(rootPath, portFiles);
    }, 10000);

    context.subscriptions.push(watcher);
}

function deactivate() {
    if (healthTimer) {
        clearInterval(healthTimer);
    }
    closeSimpleBrowserTabs();
    if (watcher) {
        watcher.dispose();
    }
}

module.exports = {
    activate,
    deactivate
}

`, DefaultPortFileName, appdb.DefaultCodraftDir)

func syncExtensionsJSON(extensionsDir string) {
	jsonPath := filepath.Join(extensionsDir, "extensions.json")
	data, err := os.ReadFile(jsonPath)
	if err != nil || len(data) == 0 {
		return
	}

	var items []map[string]interface{}
	if err := json.Unmarshal(data, &items); err != nil {
		return
	}

	extID := HelperExtPublisher + "." + HelperExtName
	relPath := HelperExtDir
	absPath := filepath.ToSlash(filepath.Join(extensionsDir, relPath))
	if !strings.HasPrefix(absPath, "/") {
		absPath = "/" + absPath
	}

	cleaned := make([]map[string]interface{}, 0, len(items)+1)
	found := false
	changed := false

	for _, item := range items {
		id := ""
		if ident, ok := item["identifier"].(map[string]interface{}); ok {
			id, _ = ident["id"].(string)
		}

		if strings.HasPrefix(id, "undefined_publisher.") || id == "task-tracker-ui-opener" || id == "codraft-ui-opener" {
			changed = true
			continue
		}

		if id == extID {
			found = true
			if version, _ := item["version"].(string); version != HelperExtVersion {
				item["version"] = HelperExtVersion
				changed = true
			}
		}

		cleaned = append(cleaned, item)
	}

	if !found {
		newItem := map[string]interface{}{
			"identifier": map[string]interface{}{
				"id": extID,
			},
			"version": HelperExtVersion,
			"location": map[string]interface{}{
				"$mid":   1,
				"path":   absPath,
				"scheme": "file",
			},
			"relativeLocation": relPath,
			"metadata": map[string]interface{}{
				"installedTimestamp": time.Now().UnixMilli(),
			},
		}
		cleaned = append(cleaned, newItem)
		changed = true
	}

	if changed {
		newData, err := json.MarshalIndent(cleaned, "", "  ")
		if err == nil {
			_ = os.WriteFile(jsonPath, newData, 0644)
		}
	}
}

func ensureExtensionInstalled() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	candidates := []string{
		filepath.Join(home, ".gemini", "antigravity-ide", "extensions"),
		filepath.Join(home, ".cursor", "extensions"),
		filepath.Join(home, ".windsurf", "extensions"),
		filepath.Join(home, ".antigravity", "extensions"),
		filepath.Join(home, ".vscode", "extensions"),
	}

	installedCount := 0
	for _, dir := range candidates {
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			continue
		}

		for _, legacy := range legacyExtNames {
			legacyDir := filepath.Join(dir, legacy)
			if _, err := os.Stat(legacyDir); err != nil {
				continue
			}
			if err := os.RemoveAll(legacyDir); err != nil {
				LogWarn("CONFIG", "Failed to remove legacy helper extension %s: %v", legacyDir, err)
			} else {
				LogInfo("CONFIG", "Removed legacy helper extension %s", legacyDir)
			}
		}

		extDir := filepath.Join(dir, HelperExtDir)
		_ = os.MkdirAll(extDir, 0755)
		changed := writeFileIfChanged(filepath.Join(extDir, "package.json"), []byte(extPackageJSON))
		if writeFileIfChanged(filepath.Join(extDir, "extension.js"), []byte(extExtensionJS)) {
			changed = true
		}

		syncExtensionsJSON(dir)

		if changed {
			installedCount++
		}
	}

	if installedCount > 0 {
		LogInfo("CONFIG", "Helper extension installed/updated in %d IDE directories", installedCount)
	}
}

func ensureMCPServerRegistered() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	lowerExe := strings.ToLower(exe)
	if strings.Contains(lowerExe, "go-build") || strings.Contains(lowerExe, ".test.exe") {
		return
	}
	exePath := filepath.ToSlash(exe)

	appData := os.Getenv("APPDATA")
	home, _ := os.UserHomeDir()

	configPaths := []string{
		filepath.Join(home, ".kilocode", "globalStorage", "kilo code.kilo-code", "settings", "mcp_settings.json"),
		filepath.Join(home, ".kilocode", "globalStorage", "kilocode.kilo-code", "settings", "mcp_settings.json"),
		filepath.Join(appData, "Code", "User", "globalStorage", "kilocode.kilo-code", "settings", "mcp_settings.json"),
		filepath.Join(appData, "Code", "User", "globalStorage", "kilo code.kilo-code", "settings", "mcp_settings.json"),
		filepath.Join(appData, "Code", "User", "globalStorage", "rooveterinaryinc.roo-cline", "settings", "mcp_settings.json"),
		filepath.Join(appData, "Code", "User", "globalStorage", "saoudrizwan.claude-dev", "settings", "mcp_settings.json"),
		filepath.Join(home, ".gemini", "antigravity", "mcp_config.json"),
	}

	for _, cfgPath := range configPaths {
		dir := filepath.Dir(cfgPath)
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			updateMCPConfigFile(cfgPath, exePath)
		}
	}
}

func updateMCPConfigFile(cfgPath, exePath string) {
	root := map[string]interface{}{}
	if data, err := os.ReadFile(cfgPath); err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &root); err != nil {
			LogWarn("CONFIG", "Skipping %s: not valid JSON (%v)", cfgPath, err)
			return
		}
	}

	servers, _ := root["mcpServers"].(map[string]interface{})
	if servers == nil {
		servers = map[string]interface{}{}
	}

	entry, _ := servers[AppName].(map[string]interface{})
	if entry == nil {
		entry = map[string]interface{}{}
		for _, legacy := range legacyAppNames {
			if old, ok := servers[legacy].(map[string]interface{}); ok {
				for k, v := range old {
					entry[k] = v
				}
				break
			}
		}
	}
	entry["command"] = exePath

	for _, legacy := range legacyAppNames {
		delete(servers, legacy)
	}

	servers[AppName] = entry
	root["mcpServers"] = servers

	newData, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		LogError("CONFIG", "Failed to encode MCP config for %s: %v", cfgPath, err)
		return
	}
	if writeFileIfChanged(cfgPath, newData) {
		LogInfo("CONFIG", "Auto-registered %s MCP server in %s", AppName, cfgPath)
	}
}

func autoRegisterIDE() {
	if os.Getenv("TRACKER_AUTO_INSTALL") == "0" {
		return
	}
	ensureExtensionInstalled()
	ensureMCPServerRegistered()
}
