package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	appdb "codraft-mcp/pkg/db"
)

func testRegistryPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ports.json")
	t.Setenv("TRACKER_REGISTRY_PATH", path)
	return path
}

func TestClientToIDE(t *testing.T) {
	cases := map[string]string{
		"kilo-code":              "vscode",
		"Kilo-Code":              "vscode",
		"kilo-code-enterprise":   "vscode",
		"kilo":                   "vscode",
		"Kilo":                   "vscode",
		"roo-cline":              "vscode",
		"roo-cline-1.5.0":        "vscode",
		"Roo Code":               "vscode",
		"roo code":               "vscode",
		"cline":                  "vscode",
		"claude-code":            "vscode",
		"Claude Code":            "vscode",
		"claude code":            "vscode",
		"github-copilot":         "vscode",
		"cursor":                 "cursor",
		"cursor-0.5":             "cursor",
		"windsurf":               "windsurf",
		"antigravity":            "antigravity",
		"antigravity-client":     "antigravity",
		"some-other-client":      "unknown",
		"":                       "unknown",
		"  kilo-code  ":          "vscode",
	}
	for name, want := range cases {
		if got := clientToIDE(name); got != want {
			t.Errorf("clientToIDE(%q) = %q; want %q", name, got, want)
		}
	}
}

func TestRegistryKeyNormalization(t *testing.T) {
	a := registryKey("kilo-code", `C:\_projects\Logicore\`)
	b := registryKey("kilo-code", `c:/_projects/logicore`)
	c := registryKey("kilo-code", `c:\_projects\LOGICORE`)
	if a != b || b != c {
		t.Fatalf("keys differ: %q vs %q vs %q", a, b, c)
	}
	if !strings.Contains(a, "kilo-code|c:/_projects/logicore") {
		t.Errorf("unexpected normalized key: %q", a)
	}

	if got := registryKey("", `C:\_projects\x`); got != "pending|c:/_projects/x" {
		t.Errorf("empty client key = %q; want pending|c:/_projects/x", got)
	}
}

func TestRegistryUpsertFindRemove(t *testing.T) {
	testRegistryPath(t)

	cwd := `C:\_projects\logicore`
	entry := RegistryEntry{
		Port:          1324,
		PID:           1001,
		Project:       "logicore",
		Cwd:           cwd,
		Client:        "kilo-code",
		ClientVersion: "7.4.5",
		IDE:           "vscode",
	}
	if err := registryUpsert(entry); err != nil {
		t.Fatalf("registryUpsert failed: %v", err)
	}

	got, ok := registryFindLive(registryKey("kilo-code", cwd))
	if !ok {
		t.Fatal("registryFindLive did not find entry")
	}
	if got.Port != 1324 || got.PID != 1001 || got.ClientVersion != "7.4.5" {
		t.Fatalf("unexpected entry: %+v", got)
	}

	if err := registryRemoveByPid(1001); err != nil {
		t.Fatalf("registryRemoveByPid failed: %v", err)
	}
	if _, ok := registryFindLive(registryKey("kilo-code", cwd)); ok {
		t.Fatal("entry still present after removal")
	}
}

func TestRegistryRemoveByPidDoesNotTouchOthers(t *testing.T) {
	testRegistryPath(t)

	cwd := `C:\_projects\a`
	for i, client := range []string{"kilo-code", "roo-cline"} {
		_ = registryUpsert(RegistryEntry{Port: 1000 + i, PID: 1001 + i, Cwd: cwd, Client: client})
	}
	if err := registryRemoveByPid(1002); err != nil {
		t.Fatalf("registryRemoveByPid failed: %v", err)
	}
	if _, ok := registryFindLive(registryKey("kilo-code", cwd)); !ok {
		t.Fatal("unrelated entry was removed")
	}
	if _, ok := registryFindLive(registryKey("roo-cline", cwd)); ok {
		t.Fatal("target entry still present")
	}
}

func TestRegistryRemoveKeyIfPid(t *testing.T) {
	testRegistryPath(t)

	cwd := `C:\_projects\a`
	_ = registryUpsert(RegistryEntry{Port: 1111, PID: 1, Cwd: cwd, Client: "kilo-code"})

	if err := registryRemoveKeyIfPid(registryKey("kilo-code", cwd), 2); err != nil {
		t.Fatalf("registryRemoveKeyIfPid failed: %v", err)
	}
	if _, ok := registryFindLive(registryKey("kilo-code", cwd)); !ok {
		t.Fatal("entry removed by foreign pid")
	}

	if err := registryRemoveKeyIfPid(registryKey("kilo-code", cwd), 1); err != nil {
		t.Fatalf("registryRemoveKeyIfPid failed: %v", err)
	}
	if _, ok := registryFindLive(registryKey("kilo-code", cwd)); ok {
		t.Fatal("entry not removed by own pid")
	}
}

func TestRegistryTTLAndCleanupStale(t *testing.T) {
	testRegistryPath(t)

	cwd := `C:\_projects\a`
	now := time.Now().UTC()
	oldTime := now.Add(-2 * RegistryTTL).Format(time.RFC3339)
	newTime := now.Format(time.RFC3339)

	_ = registryUpsert(RegistryEntry{Port: 1, PID: 1, Cwd: cwd, Client: "kilo-code", UpdatedAt: oldTime})
	_ = registryUpsert(RegistryEntry{Port: 2, PID: 2, Cwd: cwd, Client: "roo-cline", UpdatedAt: newTime})

	if _, ok := registryFindLive(registryKey("kilo-code", cwd)); ok {
		t.Fatal("stale entry reported as live")
	}
	if _, ok := registryFindLive(registryKey("roo-cline", cwd)); !ok {
		t.Fatal("fresh entry not reported as live")
	}

	if err := registryCleanupStale(); err != nil {
		t.Fatalf("registryCleanupStale failed: %v", err)
	}
	data := registrySnapshot()
	if _, ok := data[registryKey("kilo-code", cwd)]; ok {
		t.Fatal("stale entry not cleaned")
	}
	if _, ok := data[registryKey("roo-cline", cwd)]; !ok {
		t.Fatal("fresh entry removed by cleanup")
	}
}

func TestRegistryHasLiveEntryForCwd(t *testing.T) {
	testRegistryPath(t)

	if registryHasLiveEntryForCwd(`C:\_projects\a`) {
		t.Fatal("no entries yet, but has live entry")
	}

	_ = registryUpsert(RegistryEntry{Port: 1, PID: 1, Cwd: `C:\_projects\A`, Client: "kilo-code"})
	if !registryHasLiveEntryForCwd(`c:/_projects/a`) {
		t.Fatal("did not find entry for cwd with different case/slashes")
	}
	if registryHasLiveEntryForCwd(`C:\_projects\b`) {
		t.Fatal("found entry for unrelated cwd")
	}

	_ = registryUpsert(RegistryEntry{
		Port:      2,
		PID:       2,
		Cwd:       `C:\_projects\b`,
		Client:    "kilo-code",
		UpdatedAt: time.Now().UTC().Add(-2 * RegistryTTL).Format(time.RFC3339),
	})
	if registryHasLiveEntryForCwd(`C:\_projects\b`) {
		t.Fatal("stale entry counted as live")
	}
}

func TestRegistryConcurrentUpserts(t *testing.T) {
	testRegistryPath(t)

	cwd := `C:\_projects\conc`
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			client := "client-" + string(rune('a'+i))
			_ = registryUpsert(RegistryEntry{Port: 1000 + i, PID: i, Cwd: cwd, Client: client})
		}(i)
	}
	wg.Wait()

	data := registrySnapshot()
	if len(data) != 20 {
		t.Fatalf("expected 20 entries, got %d (file may be corrupted)", len(data))
	}
	for i := 0; i < 20; i++ {
		client := "client-" + string(rune('a'+i))
		if _, ok := data[registryKey(client, cwd)]; !ok {
			t.Fatalf("missing entry for %s", client)
		}
	}
}

func TestExtensionTemplateSync(t *testing.T) {
	disk, err := os.ReadFile(filepath.Join("vscode-extension", "extension.js"))
	if err != nil {
		t.Fatalf("failed to read vscode-extension/extension.js: %v", err)
	}
	expected := strings.TrimSpace(strings.ReplaceAll(
		strings.ReplaceAll(extExtensionJS, DefaultPortFileName, "PH_PORT"),
		appdb.DefaultCodraftDir, "PH_DIR",
	))
	actual := strings.TrimSpace(strings.ReplaceAll(
		strings.ReplaceAll(string(disk), "tracker.port", "PH_PORT"),
		".codraft", "PH_DIR",
	))
	expected = strings.ReplaceAll(expected, "\r\n", "\n")
	actual = strings.ReplaceAll(actual, "\r\n", "\n")
	if expected != actual {
		el := strings.Split(expected, "\n")
		al := strings.Split(actual, "\n")
		n := len(el)
		if len(al) > n {
			n = len(al)
		}
		for i := 0; i < n; i++ {
			var e, a string
			if i < len(el) {
				e = el[i]
			}
			if i < len(al) {
				a = al[i]
			}
			if e != a {
				fmt.Fprintf(os.Stderr, "LINE %d:\n  TEMPLATE: %q\n  FILE:     %q\n", i+1, e, a)
			}
		}
		t.Fatalf("vscode-extension/extension.js and config.go template are out of sync.")
	}
}
