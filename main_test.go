package main

import (
	"net"
	"testing"

	"echogallery/internal/api"
	"echogallery/internal/config"
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

func TestPrepareBatchTaskRowsForResume_KeepsCompletedRowsWhenRerunDisabled(t *testing.T) {
	libraries := []config.Library{
		{ID: "lib_a", Name: "资源库 A", Path: "/tmp/library-a"},
	}
	previous := []api.LibraryBatchBuildLibraryStatus{
		{
			ID:        "lib_a",
			Name:      "资源库 A",
			Path:      "/tmp/library-a",
			Status:    "completed",
			Message:   "已整理完成",
			Generated: 12,
			Skipped:   34,
		},
	}

	rows := prepareBatchTaskRowsForResume(previous, libraries, true, false)

	if len(rows) != 1 {
		t.Fatalf("期望 1 行资源库状态，得到 %d", len(rows))
	}
	if rows[0].Status != "completed" {
		t.Fatalf("整理选项已完成时不应重置为 pending，得到 %q", rows[0].Status)
	}
	if rows[0].Generated != 12 || rows[0].Skipped != 34 {
		t.Fatalf("已完成行的统计不应被清零，得到 generated=%d skipped=%d", rows[0].Generated, rows[0].Skipped)
	}
}

func TestPrepareBatchTaskRowsForResume_CanRerunCompletedRows(t *testing.T) {
	libraries := []config.Library{
		{ID: "lib_a", Name: "资源库 A", Path: "/tmp/library-a"},
	}
	previous := []api.LibraryBatchBuildLibraryStatus{
		{
			ID:        "lib_a",
			Name:      "资源库 A",
			Path:      "/tmp/library-a",
			Status:    "completed",
			Message:   "已完成",
			Generated: 12,
			Skipped:   34,
		},
	}

	rows := prepareBatchTaskRowsForResume(previous, libraries, true, true)

	if len(rows) != 1 {
		t.Fatalf("期望 1 行资源库状态，得到 %d", len(rows))
	}
	if rows[0].Status != "pending" {
		t.Fatalf("允许重新运行时应重置为 pending，得到 %q", rows[0].Status)
	}
	if rows[0].Generated != 0 || rows[0].Skipped != 0 {
		t.Fatalf("重新运行时应清空旧统计，得到 generated=%d skipped=%d", rows[0].Generated, rows[0].Skipped)
	}
}
