//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"
)

func acquirePlatformFileLock(file *os.File, maxAttempts int, retryDelay time.Duration) error {
	for i := 0; i < maxAttempts; i++ {
		if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			return nil
		}
		if i < maxAttempts-1 && retryDelay > 0 {
			time.Sleep(retryDelay)
		}
	}
	return fmt.Errorf("lock timeout")
}

func openBrowserPlatform(url string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("open", url)
	} else {
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
