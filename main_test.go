package main

import (
	"net"
	"testing"
)

func TestListenTCPWithFallback_PicksAnotherPortWhenPreferredBusy(t *testing.T) {
	busy, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("预占端口失败: %v", err)
	}
	defer busy.Close()

	preferredPort := busy.Addr().(*net.TCPAddr).Port
	listener, actualPort, err := listenTCPWithFallback(preferredPort)
	if err != nil {
		t.Fatalf("listenTCPWithFallback 返回错误: %v", err)
	}
	defer listener.Close()

	if actualPort == preferredPort {
		t.Fatalf("期望端口冲突时回退到其他端口，实际仍为 %d", actualPort)
	}
}
