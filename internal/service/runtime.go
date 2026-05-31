package service

import "sync/atomic"

var lowResourceModeFlag atomic.Uint32
var batchAggressiveModeFlag atomic.Uint32

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

func SetBatchAggressiveMode(enabled bool) {
	if enabled {
		batchAggressiveModeFlag.Store(1)
		return
	}
	batchAggressiveModeFlag.Store(0)
}

func batchAggressiveModeEnabled() bool {
	return batchAggressiveModeFlag.Load() == 1
}
