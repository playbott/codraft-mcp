package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	appdb "codraft-mcp/pkg/db"
)

const (
	RegistryFileName         = "ports.json"
	RegistryTTL              = 45 * time.Second
	RegistryHeartbeatInterval = 10 * time.Second
	RegistryClientPending    = "pending"
)

type RegistryEntry struct {
	Port          int    `json:"port"`
	PID           int    `json:"pid"`
	Project       string `json:"project"`
	Cwd           string `json:"cwd"`
	Client        string `json:"client"`
	ClientVersion string `json:"client_version"`
	IDE           string `json:"ide"`
	UpdatedAt     string `json:"updated_at"`
}

func registryFile() string {
	if p := os.Getenv("TRACKER_REGISTRY_PATH"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, appdb.DefaultCodraftDir, RegistryFileName)
}

func registryLockFile() string {
	return registryFile() + ".lock"
}

func registryNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func registryParseTime(value string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func entryIsAlive(e RegistryEntry, now time.Time) bool {
	t, ok := registryParseTime(e.UpdatedAt)
	if !ok {
		return false
	}
	return now.Sub(t) < RegistryTTL
}

func registryNormalizeCwd(cwd string) string {
	c := strings.ReplaceAll(cwd, "\\", "/")
	c = filepath.ToSlash(filepath.Clean(c))
	c = strings.ReplaceAll(c, "\\", "/")
	if len(c) > 1 {
		c = strings.TrimRight(c, "/")
	}
	return strings.ToLower(c)
}

func registryKey(client, cwd string) string {
	if client == "" {
		client = RegistryClientPending
	}
	return client + "|" + registryNormalizeCwd(cwd)
}

func clientToIDE(clientName string) string {
	name := strings.ToLower(strings.TrimSpace(clientName))
	compact := strings.NewReplacer("-", "", "_", "", " ", "").Replace(name)
	switch {
	case strings.HasPrefix(compact, "kilo"),
		strings.HasPrefix(compact, "roo"),
		strings.HasPrefix(compact, "cline"),
		strings.HasPrefix(compact, "claudecode"),
		strings.HasPrefix(compact, "githubcopilot"):
		return "vscode"
	case strings.HasPrefix(compact, "cursor"):
		return "cursor"
	case strings.HasPrefix(compact, "windsurf"):
		return "windsurf"
	case strings.HasPrefix(compact, "antigravity"):
		return "antigravity"
	default:
		return "unknown"
	}
}

func registryAcquireLock() (*os.File, error) {
	lPath := registryLockFile()
	_ = os.MkdirAll(filepath.Dir(lPath), 0755)
	file, err := os.OpenFile(lPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return nil, err
	}

	if err := acquirePlatformFileLock(file, 20, 25*time.Millisecond); err != nil {
		file.Close()
		return nil, fmt.Errorf("registry lock timeout on %s: %w", lPath, err)
	}
	return file, nil
}

func registrySaveLocked(data map[string]RegistryEntry) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	tmp := registryFile() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, registryFile()); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func registryWithLock(mutate func(map[string]RegistryEntry) (map[string]RegistryEntry, bool)) error {
	lockFile, err := registryAcquireLock()
	if err != nil {
		return err
	}
	defer lockFile.Close()

	data := map[string]RegistryEntry{}
	if raw, err := os.ReadFile(registryFile()); err == nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, &data); err != nil {
			LogWarn("REG", "Corrupt registry %s (%v); starting fresh", registryFile(), err)
			data = map[string]RegistryEntry{}
		}
	}

	newData, changed := mutate(data)
	if !changed {
		return nil
	}
	return registrySaveLocked(newData)
}

func registrySnapshot() map[string]RegistryEntry {
	data := map[string]RegistryEntry{}
	if raw, err := os.ReadFile(registryFile()); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &data)
	}
	return data
}

func registryUpsert(entry RegistryEntry) error {
	if entry.UpdatedAt == "" {
		entry.UpdatedAt = registryNow()
	}
	if entry.Cwd == "" {
		cwd, err := os.Getwd()
		if err == nil {
			entry.Cwd = cwd
		}
	}
	if entry.Client == "" {
		entry.Client = RegistryClientPending
	}
	if entry.IDE == "" {
		entry.IDE = clientToIDE(entry.Client)
	}
	key := registryKey(entry.Client, entry.Cwd)
	return registryWithLock(func(data map[string]RegistryEntry) (map[string]RegistryEntry, bool) {
		if entry.Client == RegistryClientPending {
			for _, e := range data {
				if e.Client != RegistryClientPending && registryNormalizeCwd(e.Cwd) == registryNormalizeCwd(entry.Cwd) {
					return data, false
				}
			}
		}
		data[key] = entry
		return data, true
	})
}

func registryTouch(key string) error {
	if key == "" {
		return nil
	}
	return registryWithLock(func(data map[string]RegistryEntry) (map[string]RegistryEntry, bool) {
		e, ok := data[key]
		if !ok {
			return data, false
		}
		e.UpdatedAt = registryNow()
		data[key] = e
		return data, true
	})
}

func registryRemoveByPid(pid int) error {
	if pid <= 0 {
		return nil
	}
	return registryWithLock(func(data map[string]RegistryEntry) (map[string]RegistryEntry, bool) {
		changed := false
		for k, e := range data {
			if e.PID == pid {
				delete(data, k)
				changed = true
			}
		}
		return data, changed
	})
}

func registryRemoveKeyIfPid(key string, pid int) error {
	if key == "" || pid <= 0 {
		return nil
	}
	return registryWithLock(func(data map[string]RegistryEntry) (map[string]RegistryEntry, bool) {
		e, ok := data[key]
		if !ok || e.PID != pid {
			return data, false
		}
		delete(data, key)
		return data, true
	})
}

func registryCleanupStale() error {
	return registryWithLock(func(data map[string]RegistryEntry) (map[string]RegistryEntry, bool) {
		now := time.Now()
		changed := false
		for k, e := range data {
			if !entryIsAlive(e, now) {
				delete(data, k)
				changed = true
			}
		}
		return data, changed
	})
}

func registryFindLive(key string) (RegistryEntry, bool) {
	e, ok := registrySnapshot()[key]
	if !ok || !entryIsAlive(e, time.Now()) {
		return RegistryEntry{}, false
	}
	return e, true
}

func registryHasLiveEntryForCwd(cwd string) bool {
	normalized := registryNormalizeCwd(cwd)
	now := time.Now()
	for _, e := range registrySnapshot() {
		if registryNormalizeCwd(e.Cwd) == normalized && entryIsAlive(e, now) {
			return true
		}
	}
	return false
}
