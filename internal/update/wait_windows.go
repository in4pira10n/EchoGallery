//go:build windows

package update

import (
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/windows"
)

func waitForPIDExit(pid int) error {
	if pid <= 0 {
		return nil
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return nil
		}
		return err
	}
	defer windows.CloseHandle(handle)
	status, err := windows.WaitForSingleObject(handle, uint32((2*time.Minute)/time.Millisecond))
	if err != nil {
		return err
	}
	switch status {
	case windows.WAIT_OBJECT_0:
		return nil
	case uint32(windows.WAIT_TIMEOUT):
		return fmt.Errorf("timeout waiting for process %d to exit", pid)
	default:
		return fmt.Errorf("unexpected wait result %d for process %d", status, pid)
	}
}
