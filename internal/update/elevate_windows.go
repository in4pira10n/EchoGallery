//go:build windows

package update

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32DLL          = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteW   = shell32DLL.NewProc("ShellExecuteW")
)

func startUpdaterElevatedWindows(updaterPath string, planPath string) error {
	if strings.TrimSpace(updaterPath) == "" {
		return fmt.Errorf("empty updater path")
	}
	if strings.TrimSpace(planPath) == "" {
		return fmt.Errorf("empty apply plan path")
	}
	verbPtr, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	filePtr, err := windows.UTF16PtrFromString(updaterPath)
	if err != nil {
		return err
	}
	paramsPtr, err := windows.UTF16PtrFromString(buildWindowsCommandLine([]string{"apply-plan", planPath}))
	if err != nil {
		return err
	}
	show := uintptr(1) // SW_SHOWNORMAL
	ret, _, callErr := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(filePtr)),
		uintptr(unsafe.Pointer(paramsPtr)),
		0,
		show,
	)
	if ret <= 32 {
		if callErr != windows.Errno(0) {
			return fmt.Errorf("ShellExecuteW failed: %w", callErr)
		}
		return fmt.Errorf("ShellExecuteW failed with code %d", ret)
	}
	return nil
}

func buildWindowsCommandLine(args []string) string {
	escaped := make([]string, 0, len(args))
	for _, arg := range args {
		escaped = append(escaped, windowsCommandArg(arg))
	}
	return strings.Join(escaped, " ")
}
