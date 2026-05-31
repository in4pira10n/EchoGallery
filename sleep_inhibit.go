package main

import "log"

type sleepInhibitor interface {
	Stop()
}

func startBestEffortSleepInhibitor(reason string) sleepInhibitor {
	inhibitor, err := startSleepInhibitor(reason)
	if err != nil {
		log.Printf("warning: failed to start sleep inhibitor: %v", err)
		return nil
	}
	return inhibitor
}
