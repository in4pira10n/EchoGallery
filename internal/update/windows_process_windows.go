//go:build windows

package update

import "syscall"

func hiddenWindowsProcessAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true}
}
