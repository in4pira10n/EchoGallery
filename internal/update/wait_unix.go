//go:build !windows

package update

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

func waitForPIDExit(pid int) error {
	if pid <= 0 {
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	deadline := time.Now().Add(2 * time.Minute)
	for {
		err = proc.Signal(syscall.Signal(0))
		if err == nil {
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for process %d to exit", pid)
			}
			time.Sleep(150 * time.Millisecond)
			continue
		}
		if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return nil
	}
}
