package service

import "sync/atomic"

var lowResourceModeFlag atomic.Uint32

func SetLowResourceMode(enabled bool) {
	if enabled {
		lowResourceModeFlag.Store(1)
		return
	}
	lowResourceModeFlag.Store(0)
}

func lowResourceModeEnabled() bool {
	return lowResourceModeFlag.Load() == 1
}
