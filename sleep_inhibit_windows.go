//go:build windows

package main

import (
	"fmt"
	"syscall"
)

const (
	esContinuous       = 0x80000000
	esSystemRequired   = 0x00000001
	esAwaymodeRequired = 0x00000040
)

type windowsSleepInhibitor struct{}

func (w *windowsSleepInhibitor) Stop() {
	_, _, _ = procSetThreadExecutionState.Call(uintptr(esContinuous))
}

var (
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procSetThreadExecutionState = kernel32.NewProc("SetThreadExecutionState")
)

func startSleepInhibitor(reason string) (sleepInhibitor, error) {
	flags := uintptr(esContinuous | esSystemRequired | esAwaymodeRequired)
	result, _, callErr := procSetThreadExecutionState.Call(flags)
	if result == 0 {
		if callErr != syscall.Errno(0) {
			return nil, fmt.Errorf("SetThreadExecutionState failed: %w", callErr)
		}
		return nil, fmt.Errorf("SetThreadExecutionState failed")
	}
	return &windowsSleepInhibitor{}, nil
}
