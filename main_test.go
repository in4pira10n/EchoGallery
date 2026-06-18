package main

import (
	"net"
	"path/filepath"
	"strings"
	"testing"

	"echogallery/internal/api"
	"echogallery/internal/config"
	"echogallery/internal/sessionlock"
)

func TestListenTCPWithFallback_PicksAnotherPortWhenPreferredBusy(t *testing.T) {
	busy, err := net.Listen("tcp", ":0")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "operation not permitted") {
			t.Skipf("当前环境不允许监听本地端口: %v", err)
		}
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

func TestPrepareBatchTaskRowsForResume_RerunsCompletedRowsInPartialBatch(t *testing.T) {
	libraries := []config.Library{
		{ID: "lib_a", Name: "资源库 A", Path: "/tmp/library-a"},
		{ID: "lib_b", Name: "资源库 B", Path: "/tmp/library-b"},
	}
	previous := []api.LibraryBatchBuildLibraryStatus{
		{ID: "lib_a", Name: "资源库 A", Path: "/tmp/library-a", Status: "completed", Generated: 12, Skipped: 34},
		{ID: "lib_b", Name: "资源库 B", Path: "/tmp/library-b", Status: "pending", Generated: 0, Skipped: 0},
	}

	rows := prepareBatchTaskRowsForResume(previous, libraries, true, true)

	if len(rows) != 2 {
		t.Fatalf("期望 2 行资源库状态，得到 %d", len(rows))
	}
	for _, row := range rows {
		if row.Status != "pending" {
			t.Fatalf("重新运行时所有选中资源库都应重置为 pending，%s 得到 %q", row.Name, row.Status)
		}
		if row.Generated != 0 || row.Skipped != 0 {
			t.Fatalf("重新运行时应清空旧统计，%s 得到 generated=%d skipped=%d", row.Name, row.Generated, row.Skipped)
		}
	}
}

func TestLoadPersistedBatchStatus_UsesGlobalLibrariesWhenProfileLibrariesAreEmpty(t *testing.T) {
	cfg := &config.Config{}
	libraries := []config.Library{
		{ID: "lib_a", Name: "资源库 A", Path: "/tmp/library-a"},
		{ID: "lib_b", Name: "资源库 B", Path: "/tmp/library-b"},
	}

	status := loadPersistedBatchStatus(cfg, libraries, batchTaskKindScan, false, "idle")
	selected := selectedLibrariesFromBatchStatus(libraries, status)

	if len(status.SelectedLibraryIDs) != 2 {
		t.Fatalf("期望默认勾选全局资源库，得到 %#v", status.SelectedLibraryIDs)
	}
	if len(selected) != 2 {
		t.Fatalf("期望批量任务能从全局资源库恢复可选项，得到 %d", len(selected))
	}
}

func TestBatchLibraryOwnerUserID_PrefersLibraryOwner(t *testing.T) {
	cfg := &config.Config{
		ActiveProfile: "admin",
		Users: []config.User{
			{Username: "admin", Role: config.UserRoleAdmin},
			{Username: "owner", Role: config.UserRoleAdmin},
		},
	}

	got := batchLibraryOwnerUserID(cfg, config.Library{
		ID:            "lib_owner",
		Name:          "Owner Library",
		Path:          "/tmp/owner-library",
		OwnerUsername: "owner",
	}, 0)

	if got != 2 {
		t.Fatalf("期望 root 批量任务使用资源库 owner userID=2，得到 %d", got)
	}
}

func TestFilterLibrariesLockedByOtherAdmins_RecoversAfterSessionRelease(t *testing.T) {
	store, err := sessionlock.New(filepath.Join(t.TempDir(), "locks.db"))
	if err != nil {
		t.Fatalf("创建锁数据库失败: %v", err)
	}
	defer store.Close()

	libraries := []config.Library{
		{ID: "lib_a", Name: "资源库 A", Path: "/tmp/library-a"},
		{ID: "lib_b", Name: "资源库 B", Path: "/tmp/library-b"},
	}
	if _, err := store.Acquire(libraries[0], "alice", "admin", "old-session", "browse"); err != nil {
		t.Fatalf("写入旧浏览锁失败: %v", err)
	}

	filtered := filterLibrariesLockedByOtherAdmins(store, "bob", "new-session", libraries)
	if len(filtered) != 1 || filtered[0].ID != "lib_b" {
		t.Fatalf("期望旧会话锁暂时遮挡 lib_a，得到 %+v", filtered)
	}

	if err := store.ReleaseSession("old-session"); err != nil {
		t.Fatalf("释放旧会话锁失败: %v", err)
	}
	filtered = filterLibrariesLockedByOtherAdmins(store, "bob", "new-session", libraries)
	if len(filtered) != 2 {
		t.Fatalf("期望释放旧会话后资源库全部恢复，得到 %+v", filtered)
	}
}
