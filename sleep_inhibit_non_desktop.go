//go:build !darwin && !windows

package main

type noopSleepInhibitor struct{}

func (n *noopSleepInhibitor) Stop() {}

func startSleepInhibitor(reason string) (sleepInhibitor, error) {
	return &noopSleepInhibitor{}, nil
}
