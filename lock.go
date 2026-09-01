package main

import (
	"os"
	"path/filepath"
	"time"
)

func lockFilePath() string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	vscDir := filepath.Join(cwd, ".vscode")
	_ = os.MkdirAll(vscDir, 0755)
	return filepath.Join(vscDir, "tracker.lock")
}

func stopFilePath() string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	vscDir := filepath.Join(cwd, ".vscode")
	_ = os.MkdirAll(vscDir, 0755)
	return filepath.Join(vscDir, "tracker.stop")
}

type ProjectLock struct {
	file *os.File
}

func tryAcquireProjectLock() (*ProjectLock, bool) {
	lPath := lockFilePath()
	file, err := os.OpenFile(lPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return nil, false
	}

	if err := acquirePlatformFileLock(file, 1, 0); err != nil {
		file.Close()
		return nil, false
	}

	_ = os.Remove(stopFilePath())

	return &ProjectLock{file: file}, true
}

func (l *ProjectLock) Release() {
	if l != nil && l.file != nil {
		_ = l.file.Close()
		_ = os.Remove(lockFilePath())
	}
}

func sendHandoverSignal() {
	_ = os.WriteFile(stopFilePath(), []byte("stop"), 0644)
	LogInfo("LOCK", "Sent handover stop signal to existing process via %s", stopFilePath())
}

func watchHandoverSignal(onStop func()) {
	go func() {
		ticker := time.NewTicker(400 * time.Millisecond)
		defer ticker.Stop()

		sFile := stopFilePath()
		for range ticker.C {
			if _, err := os.Stat(sFile); err == nil {
				LogWarn("LOCK", "Received handover stop signal from new process. Gracefully exiting...")
				_ = os.Remove(sFile)
				onStop()
				os.Exit(0)
			}
		}
	}()
}
