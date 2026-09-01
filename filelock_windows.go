//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
	"unsafe"
)

var (
	modkernel32    = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx = modkernel32.NewProc("LockFileEx")
)

const (
	lockfileExclusiveLock   = 0x00000002
	lockfileFailImmediately = 0x00000001
)

func lockFileEx(h syscall.Handle, flags, reserved, lockLow, lockHigh uint32, overlapped *syscall.Overlapped) error {
	r1, _, err := procLockFileEx.Call(
		uintptr(h),
		uintptr(flags),
		uintptr(reserved),
		uintptr(lockLow),
		uintptr(lockHigh),
		uintptr(unsafe.Pointer(overlapped)),
	)
	if r1 == 0 {
		if err != nil && err != syscall.Errno(0) {
			return err
		}
		return syscall.EINVAL
	}
	return nil
}

func acquirePlatformFileLock(file *os.File, maxAttempts int, retryDelay time.Duration) error {
	var overlapped syscall.Overlapped
	h := syscall.Handle(file.Fd())
	flags := uint32(lockfileExclusiveLock | lockfileFailImmediately)

	for i := 0; i < maxAttempts; i++ {
		if err := lockFileEx(h, flags, 0, 1, 0, &overlapped); err == nil {
			return nil
		}
		if i < maxAttempts-1 && retryDelay > 0 {
			time.Sleep(retryDelay)
		}
	}
	return fmt.Errorf("lock timeout")
}

func openBrowserPlatform(url string) error {
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	return cmd.Start()
}
