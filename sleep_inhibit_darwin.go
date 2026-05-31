//go:build darwin

package main

import (
	"context"
	"fmt"
	"os/exec"
)

type commandSleepInhibitor struct {
	cancel context.CancelFunc
}

func (c *commandSleepInhibitor) Stop() {
	if c == nil || c.cancel == nil {
		return
	}
	c.cancel()
}

func startSleepInhibitor(reason string) (sleepInhibitor, error) {
	ctx, cancel := context.WithCancel(context.Background())
	// `-i` prevents idle sleep, `-m` prevents disk idle sleep, `-s` prevents system sleep on AC.
	cmd := exec.CommandContext(ctx, "caffeinate", "-i", "-m", "-s")
	if err := cmd.Start(); err != nil {
		cancel()
		if reason != "" {
			return nil, fmt.Errorf("%s: %w", reason, err)
		}
		return nil, err
	}
	go func() {
		_ = cmd.Wait()
	}()
	return &commandSleepInhibitor{cancel: cancel}, nil
}
