package main

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"echogallery/internal/api"
	"echogallery/internal/config"
	imgpkg "echogallery/internal/image"
	"echogallery/internal/sessionlock"
	"echogallery/internal/storage"
	"echogallery/internal/storage/sqlite"
)

func TestBatchLegacyThumbnailMigrationRestoresUUIDAndCopiesFile(t *testing.T) {
	base := t.TempDir()
	library := config.Library{ID: "lib_test", Name: "Test", Path: filepath.Join(base, "library")}
	if err := os.MkdirAll(library.Path, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{AppDataDir: filepath.Join(base, "echogallery-data"), StoragePath: library.Path}
	cfg.ThumbnailDir = filepath.Join(cfg.AppDataDir, "thumbnails")
	oldPath := cfg.LegacyDatabasePathForStorage(library.Path)
	if err := os.MkdirAll(filepath.Dir(oldPath), 0755); err != nil {
		t.Fatal(err)
	}
	old, err := sqlite.New(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	oldID := "35bd948d-019e-435d-bf41-01c63dd4d945"
	photo := func(id string) *storage.Photo {
		return &storage.Photo{
			UUID: id, OriginalName: "a.jpg", MediaKind: storage.MediaKindImage,
			MimeType: "image/jpeg", Size: 128, SourceRelPath: "a.jpg",
			TakenAt: time.Now(), UploadedAt: time.Now(), UploadedBy: 1,
		}
	}
	if err := old.SavePhoto(photo(oldID)); err != nil {
		t.Fatal(err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	oldThumbnail := imgpkg.ThumbnailShardPath(filepath.Join(cfg.ThumbnailDir, library.ID), oldID)
	var jpegBytes bytes.Buffer
	if err := jpeg.Encode(&jpegBytes, image.NewRGBA(image.Rect(0, 0, 40, 30)), nil); err != nil {
		t.Fatal(err)
	}
	if err := imgpkg.GenerateThumbnail(bytes.NewReader(jpegBytes.Bytes()), "image/jpeg", oldThumbnail, 64); err != nil {
		t.Fatal(err)
	}
	mediaDir, trashDir, thumbDir, err := prepareLibraryBatchPaths(cfg, library)
	if err != nil {
		t.Fatal(err)
	}
	repo, svc, err := openBatchLibraryService(cfg, library, mediaDir, trashDir, thumbDir)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.SavePhoto(photo("22222222-2222-4222-8222-222222222222")); err != nil {
		t.Fatal(err)
	}
	svc.SetLegacyThumbnailRoot(cfg.ThumbnailDir)
	matches, err := repo.RestoreLegacyUUIDs(context.Background(), oldPath)
	if err != nil || len(matches) != 1 {
		t.Fatalf("旧 UUID 映射失败: %+v, %v", matches, err)
	}
	result, err := svc.MigrateLegacyThumbnailFiles(context.Background(), matches, nil, nil)
	if err != nil || result.Copied != 1 {
		t.Fatalf("复制旧缩略图失败: %+v, %v", result, err)
	}
	missing, err := svc.AuditNewThumbnails(context.Background(), 1)
	if err != nil || missing != 0 {
		t.Fatalf("新缩略图核对失败: missing=%d, err=%v", missing, err)
	}
	if _, err := os.Stat(oldThumbnail); err != nil {
		t.Fatalf("迁移不应删除旧缩略图: %v", err)
	}
	if got, err := repo.GetPhotoBySourceRelPath("a.jpg", 1); err != nil || got.UUID != oldID {
		t.Fatalf("新版媒体 UUID 不正确: %+v, %v", got, err)
	}
}

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

func TestBatchProgressCountsDoesNotCountActiveLibraryAsCompleted(t *testing.T) {
	status := api.LibraryBatchBuildStatus{Status: "running", TotalLibraries: 2, CurrentLibraryIndex: 1, CurrentPercent: 50}
	done, total := batchProgressCounts(status)
	if done != 0 || total != 2 {
		t.Fatalf("当前资源库进行中时应显示 0/2，得到 %d/%d", done, total)
	}
	if got := batchOverallPercent(status); got != 25 {
		t.Fatalf("首库处理一半时总进度应为 25%%，得到 %.1f", got)
	}
}

func TestBatchThumbnailStartRejectsPausedMigration(t *testing.T) {
	base := t.TempDir()
	library := config.Library{ID: "lib_test", Name: "Test", Path: filepath.Join(base, "library")}
	if err := os.MkdirAll(library.Path, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		AppDataDir:  filepath.Join(base, "echogallery-data"),
		StoragePath: library.Path,
		Libraries:   []config.Library{library},
		BatchThumbnails: config.BatchTaskState{
			Status: "completed", SelectionConfigured: true,
			SelectedLibraryIDs: []string{library.ID}, MoveLegacyThumbnails: true,
			Libraries: []config.BatchTaskLibraryState{{ID: library.ID, Name: library.Name, Path: library.Path, Status: "completed"}},
		},
	}
	legacyPath := cfg.LegacyDatabasePathForStorage(library.Path)
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, []byte("old database"), 0644); err != nil {
		t.Fatal(err)
	}
	newPath, err := cfg.DatabasePathForStorage(library.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
		t.Fatal(err)
	}
	repo, err := sqlite.New(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveLegacyThumbnailRun(context.Background(), legacyPath, false); err != nil {
		t.Fatal(err)
	}
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}
	manager := newLibraryBatchThumbnailBuildManager(nil, nil, "")
	status, err := manager.Start(cfg, &config.Profile{}, "alice", 1, false, true, false, false)
	if err == nil || manager.task != nil {
		t.Fatalf("迁移暂停期间不能创建任务: status=%+v err=%v", status, err)
	}
}

func TestBatchScanStartRejectsPausedMigration(t *testing.T) {
	cfg := &config.Config{StoragePath: t.TempDir(), AppDataDir: t.TempDir()}
	manager := newLibraryBatchBuildManager(nil, nil, "")
	_, err := manager.Start(cfg, &config.Profile{}, "alice", 1, false, true, false, true, false, false)
	if err == nil || manager.task != nil {
		t.Fatalf("扫描任务也应拒绝迁移参数: %v", err)
	}
}

func TestBatchScanProgressReportsDiscoveryAndNamedStages(t *testing.T) {
	library := config.Library{ID: "lib_a", Name: "Library A", Path: t.TempDir()}
	task := newLibraryBatchBuildTask([]config.Library{library}, false, false, nil)
	if got := task.snapshot().CurrentPhase; got != "preparing" {
		t.Fatalf("new tasks should report preparation, got %q", got)
	}
	manager := newLibraryBatchBuildManager(nil, nil, "")
	manager.beginLibrary(task, 0, library)
	if got := task.snapshot(); got.CurrentPhase != "checking" || got.CurrentLibraryName != library.Name {
		t.Fatalf("snapshot checks must identify the current library: %+v", got)
	}
	for _, step := range []struct {
		phase       string
		done, total int
		wantPhase   string
		wantPercent float64
	}{
		{"scan", 32, 0, "discovering", 0},
		{"scan", 4, 10, "scan", 40},
		{"exif", 1, 4, "exif", 25},
	} {
		setBatchScanLibraryProgress(task, 0, library, step.phase, step.done, step.total)
		got := task.snapshot()
		if got.Status != "running" || got.CurrentPhase != step.wantPhase || got.CurrentPercent != step.wantPercent ||
			got.CurrentDone != step.done || got.CurrentTotal != step.total || got.CurrentLibraryName != library.Name ||
			got.Libraries[0].Status != "scanning" || !strings.Contains(got.Message, library.Name) {
			t.Fatalf("incorrect progress for phase %q: %+v", step.wantPhase, got)
		}
		if phase := batchPhaseLabel(got.CurrentPhase); phase == "preparing" || phase == "idle" {
			t.Fatalf("CLI lost phase %q: %q", got.CurrentPhase, phase)
		}
	}
	thumbnails := newLibraryBatchThumbnailBuildManager(nil, nil, "")
	thumbnails.beginLibrary(task, 0, library)
	if got := task.snapshot(); got.CurrentPhase != "thumbnails" || got.CurrentLibraryName != library.Name || got.Libraries[0].Status != "building" {
		t.Fatalf("thumbnail tasks must identify the current library: %+v", got)
	}
}

func TestBatchScanRechecksCompletedInactiveLibrary(t *testing.T) {
	base := t.TempDir()
	libraries := []config.Library{
		{ID: "lib_a", Name: "Library A", Path: filepath.Join(base, "a"), OwnerUsername: "admin"},
		{ID: "lib_b", Name: "Library B", Path: filepath.Join(base, "b"), OwnerUsername: "admin"},
	}
	cfg := &config.Config{
		AppDataDir: filepath.Join(base, "data"), ThumbnailDir: filepath.Join(base, "thumbnails"), ThumbnailSize: 64,
		Libraries: libraries, ActiveProfile: "admin", ActiveLibraryID: "lib_b", StoragePath: libraries[1].Path,
		Users: []config.User{{Username: "admin", Role: config.UserRoleAdmin}},
	}
	profile := &config.Profile{ActiveLibraryID: "lib_b", StoragePath: libraries[1].Path}
	var media bytes.Buffer
	if err := jpeg.Encode(&media, image.NewRGBA(image.Rect(0, 0, 16, 16)), nil); err != nil {
		t.Fatal(err)
	}
	writeMedia := func(rel string) {
		t.Helper()
		path := filepath.Join(libraries[0].Path, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, media.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
	}
	manager := newLibraryBatchBuildManager(nil, nil, "")
	completed := make(chan struct{}, 1)
	manager.afterScanComplete = func(*config.Config, string, int64, bool, bool, bool, bool, bool) error {
		completed <- struct{}{}
		return nil
	}
	run := func(completeEXIF bool) api.LibraryBatchBuildStatus {
		t.Helper()
		if _, err := manager.Start(cfg, profile, "admin", 1, false, true, completeEXIF, false, false, false); err != nil {
			t.Fatal(err)
		}
		select {
		case <-completed:
		case <-time.After(15 * time.Second):
			_, _ = manager.Cancel(cfg, "admin")
			t.Fatalf("batch scan did not reach the thumbnail stage: %+v", manager.Status(cfg, profile, "admin"))
		}
		status := manager.Status(cfg, profile, "admin")
		if status.Status != "completed" || status.CompletedLibraries != 2 || status.FailedLibraries != 0 {
			t.Fatalf("unexpected batch result: %+v", status)
		}
		if cfg.ActiveLibraryID != "lib_b" {
			t.Fatalf("batch scan changed the active library: %q", cfg.ActiveLibraryID)
		}
		return status
	}
	writeMedia("old.jpg")
	run(false)
	writeMedia("copied/nested/new.jpg")
	if err := os.Remove(filepath.Join(libraries[0].Path, "old.jpg")); err != nil {
		t.Fatal(err)
	}
	status := run(false)
	if row := status.Libraries[0]; row.Imported != 1 || row.Pruned != 1 || row.MessageKey != "" {
		t.Fatalf("inactive library changes were skipped or hidden by a stale message: %+v", row)
	}
	if status.MessageKey != "" || status.Libraries[1].MessageKey != "batchWorkflow.scanSkipped" {
		t.Fatalf("batch result must distinguish changed and unchanged libraries: %+v", status)
	}
	mediaDir, trashDir, thumbDir, err := prepareLibraryBatchPaths(cfg, libraries[0])
	if err != nil {
		t.Fatal(err)
	}
	repo, svc, err := openBatchLibraryService(cfg, libraries[0], mediaDir, trashDir, thumbDir)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	userID := svc.LibraryUserID(1)
	if photo, err := repo.GetPhotoBySourceRelPath("copied/nested/new.jpg", userID); err != nil || photo == nil {
		t.Fatalf("new media must be indexed before the thumbnail stage: photo=%+v err=%v", photo, err)
	}
	page, err := svc.GetTimeline(storage.ListPhotosParams{UserID: userID, Limit: 10})
	if err != nil || len(page.Photos) != 1 || page.Photos[0].SourceRelPath != "copied/nested/new.jpg" {
		t.Fatalf("timeline must show the new media and hide the removed media: page=%+v err=%v", page, err)
	}
	snapshot, err := repo.GetLibraryScanSnapshot()
	if err != nil || snapshot == nil || snapshot.Version != 2 {
		t.Fatalf("snapshot version must remain 2: snapshot=%+v err=%v", snapshot, err)
	}
	status = run(true)
	if status.MessageKey != "batchWorkflow.scanSkippedExifCompleted" {
		t.Fatalf("unchanged libraries should skip imports but still run the selected EXIF stage: %+v", status)
	}
	for _, row := range status.Libraries {
		if row.Imported != 0 || row.Pruned != 0 {
			t.Fatalf("unchanged library repeated stale work or counts: %+v", row)
		}
	}
	if row := status.Libraries[0]; row.MessageKey != "batchWorkflow.scanSkippedExifCompleted" || row.EXIFScanned != 2 {
		t.Fatalf("an unchanged library with the EXIF option enabled must still report its metadata pass: %+v", row)
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
