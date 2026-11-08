//go:build !windows && !darwin && !linux

package service

func platformSMBPath(path string) bool { return false }
