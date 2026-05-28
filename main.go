package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"echogallery/internal/api"
	"echogallery/internal/config"
	"echogallery/internal/service"
	"echogallery/internal/storage/sqlite"
)

const restartDelayEnv = "ECHOGALLERY_RESTART_DELAY_MS"
const portFallbackScanLimit = 20

var shutdownRequested = make(chan struct{}, 1)

type libraryBuildState struct {
	mu                sync.Mutex
	status            api.LibraryBuildStatus
	shutdownAfterDone func() error
	cancelBuild       context.CancelFunc
}

type libraryBatchBuildTask struct {
	cancel context.CancelFunc

	mu     sync.Mutex
	status api.LibraryBatchBuildStatus
}

type libraryBatchBuildManager struct {
	mu                sync.Mutex
	task              *libraryBatchBuildTask
	shutdownAfterDone func() error
	defaultExitAfter  bool
}

func newLibraryBuildState(shutdown func() error) *libraryBuildState {
	return &libraryBuildState{
		shutdownAfterDone: shutdown,
		status: api.LibraryBuildStatus{
			Status:  "idle",
			Message: "当前没有扫描任务",
		},
	}
}

func newLibraryBatchBuildManager(shutdown func() error) *libraryBatchBuildManager {
	return &libraryBatchBuildManager{shutdownAfterDone: shutdown}
}

func newLibraryBatchBuildTask(libraries []config.Library, fast bool, lowResource bool, cancel context.CancelFunc) *libraryBatchBuildTask {
	items := make([]api.LibraryBatchBuildLibraryStatus, 0, len(libraries))
	for _, library := range libraries {
		items = append(items, api.LibraryBatchBuildLibraryStatus{
			Name:    library.Name,
			Path:    library.Path,
			Status:  "pending",
			Message: "等待处理中",
		})
	}
	now := time.Now()
	return &libraryBatchBuildTask{
		cancel: cancel,
		status: api.LibraryBatchBuildStatus{
			Status:             "running",
			Message:            "正在准备批量构建资源库…",
			TotalLibraries:     len(libraries),
			FastThumbnailBuild: fast,
			LowResourceMode:    lowResource,
			ExitAfterComplete:  false,
			StartedAt:          now.Format(time.RFC3339),
			UpdatedAt:          now.Format(time.RFC3339),
			Libraries:          items,
		},
	}
}

func (t *libraryBatchBuildTask) mutate(fn func(status *api.LibraryBatchBuildStatus)) {
	if t == nil || fn == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	fn(&t.status)
	t.status.UpdatedAt = time.Now().Format(time.RFC3339)
}

func (t *libraryBatchBuildTask) snapshot() api.LibraryBatchBuildStatus {
	if t == nil {
		return api.LibraryBatchBuildStatus{Status: "idle", Message: "当前没有批量构建任务"}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	status := t.status
	if len(status.Libraries) > 0 {
		status.Libraries = append([]api.LibraryBatchBuildLibraryStatus(nil), status.Libraries...)
	}
	if status.StartedAt != "" {
		startedAt, err := time.Parse(time.RFC3339, status.StartedAt)
		if err == nil {
			end := time.Now()
			if status.FinishedAt != "" {
				if finishedAt, parseErr := time.Parse(time.RFC3339, status.FinishedAt); parseErr == nil {
					end = finishedAt
				}
			}
			status.ElapsedSeconds = int64(end.Sub(startedAt).Seconds())
		}
	}
	return status
}

func (m *libraryBatchBuildManager) Status() api.LibraryBatchBuildStatus {
	m.mu.Lock()
	task := m.task
	defaultExitAfter := m.defaultExitAfter
	m.mu.Unlock()
	if task == nil {
		return api.LibraryBatchBuildStatus{Status: "idle", Message: "当前没有批量构建任务", ExitAfterComplete: defaultExitAfter}
	}
	return task.snapshot()
}

func (m *libraryBatchBuildManager) Start(cfg *config.Config, profile *config.Profile, userID int64) (api.LibraryBatchBuildStatus, error) {
	if cfg == nil || profile == nil {
		return api.LibraryBatchBuildStatus{Status: "idle", Message: "当前没有批量构建任务"}, fmt.Errorf("批量构建参数无效")
	}
	libraries := append([]config.Library(nil), profile.Libraries...)
	if len(libraries) == 0 {
		return api.LibraryBatchBuildStatus{Status: "idle", Message: "当前没有批量构建任务"}, fmt.Errorf("当前没有可构建的资源库")
	}
	m.mu.Lock()
	defaultExitAfter := m.defaultExitAfter
	if m.task != nil {
		current := m.task.snapshot()
		if current.Status == "running" || current.Status == "cancelling" {
			m.mu.Unlock()
			return current, nil
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	task := newLibraryBatchBuildTask(libraries, profile.Preferences.FastThumbnailBuild, profile.Preferences.LowResourceMode, cancel)
	task.status.ExitAfterComplete = defaultExitAfter
	m.task = task
	cfgSnapshot := *cfg
	cfgSnapshot.ApplyProfile(profile)
	m.mu.Unlock()

	go m.run(ctx, task, &cfgSnapshot, userID)
	return task.snapshot(), nil
}

func (m *libraryBatchBuildManager) Cancel() (api.LibraryBatchBuildStatus, error) {
	m.mu.Lock()
	task := m.task
	m.mu.Unlock()
	if task == nil {
		return api.LibraryBatchBuildStatus{Status: "idle", Message: "当前没有批量构建任务"}, nil
	}
	task.mutate(func(status *api.LibraryBatchBuildStatus) {
		if status.Status == "running" {
			status.Status = "cancelling"
			status.Message = "正在取消批量构建资源库…"
		}
	})
	if task.cancel != nil {
		task.cancel()
	}
	return task.snapshot(), nil
}

func (m *libraryBatchBuildManager) SetExitAfterComplete(enabled bool) (api.LibraryBatchBuildStatus, error) {
	m.mu.Lock()
	m.defaultExitAfter = enabled
	task := m.task
	m.mu.Unlock()
	if task == nil {
		return api.LibraryBatchBuildStatus{
			Status:            "idle",
			Message:           "当前没有批量构建任务",
			ExitAfterComplete: enabled,
		}, nil
	}
	task.mutate(func(status *api.LibraryBatchBuildStatus) {
		status.ExitAfterComplete = enabled
	})
	snapshot := task.snapshot()
	if enabled && (snapshot.Status == "completed" || snapshot.Status == "failed" || snapshot.Status == "cancelled") && m.shutdownAfterDone != nil {
		_ = m.shutdownAfterDone()
	}
	return snapshot, nil
}

func (m *libraryBatchBuildManager) clear(task *libraryBatchBuildTask) {
	m.mu.Lock()
	if m.task == task {
		m.task = nil
	}
	m.mu.Unlock()
}

func (m *libraryBatchBuildManager) run(ctx context.Context, task *libraryBatchBuildTask, cfg *config.Config, userID int64) {
	if cfg == nil {
		task.mutate(func(status *api.LibraryBatchBuildStatus) {
			status.Status = "failed"
			status.Message = "批量构建失败"
			status.Error = "配置不可用"
			status.FinishedAt = time.Now().Format(time.RFC3339)
		})
		return
	}

	service.SetLowResourceMode(cfg.Preferences.LowResourceMode)

	managedDataDir, err := cfg.ManagedDataDir()
	if err != nil {
		m.failTask(task, fmt.Errorf("计算媒体数据目录失败: %w", err))
		return
	}
	trashDir, err := cfg.TrashPath()
	if err != nil {
		m.failTask(task, fmt.Errorf("计算回收站目录失败: %w", err))
		return
	}
	thumbDir, err := cfg.ThumbnailStoragePath()
	if err != nil {
		m.failTask(task, fmt.Errorf("计算缩略图目录失败: %w", err))
		return
	}
	if err := os.MkdirAll(managedDataDir, 0755); err != nil {
		m.failTask(task, fmt.Errorf("创建媒体数据目录失败: %w", err))
		return
	}
	if err := os.MkdirAll(trashDir, 0755); err != nil {
		m.failTask(task, fmt.Errorf("创建回收站目录失败: %w", err))
		return
	}
	if err := os.MkdirAll(thumbDir, 0755); err != nil {
		m.failTask(task, fmt.Errorf("创建缩略图目录失败: %w", err))
		return
	}

	libraries := append([]config.Library(nil), cfg.Libraries...)
	for index, library := range libraries {
		if ctx.Err() != nil {
			m.cancelTask(task, index)
			return
		}
		m.beginLibrary(task, index, library)
		if err := os.MkdirAll(library.Path, 0755); err != nil {
			m.failLibrary(task, index, fmt.Errorf("创建资源库目录失败: %w", err))
			continue
		}
		dbPath, err := cfg.DatabasePathForStorage(library.Path)
		if err != nil {
			m.failLibrary(task, index, fmt.Errorf("计算数据库路径失败: %w", err))
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
			m.failLibrary(task, index, fmt.Errorf("创建数据库目录失败: %w", err))
			continue
		}
		repo, err := sqlite.New(dbPath)
		if err != nil {
			m.failLibrary(task, index, fmt.Errorf("打开数据库失败: %w", err))
			continue
		}

		svc := service.NewPhotoServiceWithoutWarmup(repo, library.Path, managedDataDir, trashDir)
		svc.SetThumbnailRoot(thumbDir)
		svc.SetThumbnailSize(cfg.ThumbnailSize)

		summary, err := svc.ImportExistingPhotosContext(ctx, userID, func(done, total int) {
			task.mutate(func(status *api.LibraryBatchBuildStatus) {
				status.Message = fmt.Sprintf("正在扫描资源库 %s", library.Name)
				status.CurrentLibraryIndex = index + 1
				status.CurrentLibraryName = library.Name
				status.CurrentLibraryPath = library.Path
				status.CurrentPhase = "scan"
				status.CurrentDone = done
				status.CurrentTotal = total
				if total > 0 {
					status.CurrentPercent = float64(done) / float64(total) * 100
				} else {
					status.CurrentPercent = 0
				}
				status.Libraries[index].Status = "scanning"
				status.Libraries[index].Message = fmt.Sprintf("扫描中 %d / %d", done, total)
			})
		})
		if err != nil {
			_ = repo.Close()
			if ctx.Err() != nil {
				m.cancelTask(task, index)
				return
			}
			m.failLibrary(task, index, fmt.Errorf("扫描资源库失败: %w", err))
			continue
		}

		task.mutate(func(status *api.LibraryBatchBuildStatus) {
			status.Libraries[index].Imported = summary.Imported
			status.Libraries[index].Skipped = summary.Skipped
			status.Libraries[index].Pruned = summary.Pruned
			status.Libraries[index].Status = "building"
			status.Libraries[index].Message = "正在生成缩略图"
			status.Message = fmt.Sprintf("正在为资源库 %s 生成缩略图", library.Name)
			status.CurrentPhase = "thumbnails"
			status.CurrentDone = 0
			status.CurrentTotal = 0
			status.CurrentPercent = 0
		})

		if _, err := svc.StartThumbnailBuild(userID, cfg.Preferences.FastThumbnailBuild); err != nil {
			_ = repo.Close()
			m.failLibrary(task, index, fmt.Errorf("启动缩略图任务失败: %w", err))
			continue
		}

		thumbStatus, cancelled := waitThumbnailBuildTask(ctx, svc, task, index, library, userID)
		_ = repo.Close()
		if cancelled {
			m.cancelTask(task, index)
			return
		}
		if thumbStatus.Status != "completed" {
			failureMessage := strings.TrimSpace(thumbStatus.Error)
			if failureMessage == "" {
				failureMessage = strings.TrimSpace(thumbStatus.Message)
			}
			m.failLibrary(task, index, fmt.Errorf("%s", failureMessage))
			continue
		}
		task.mutate(func(status *api.LibraryBatchBuildStatus) {
			status.CompletedLibraries++
			status.Libraries[index].Generated = thumbStatus.Generated
			status.Libraries[index].Failed = thumbStatus.Failed
			status.Libraries[index].Status = "completed"
			status.Libraries[index].Message = thumbStatus.Message
			status.Message = fmt.Sprintf("资源库 %s 构建完成", library.Name)
			status.CurrentDone = thumbStatus.Done
			status.CurrentTotal = thumbStatus.Total
			status.CurrentPercent = thumbStatus.Percent
		})
	}

	task.mutate(func(status *api.LibraryBatchBuildStatus) {
		status.FinishedAt = time.Now().Format(time.RFC3339)
		status.CurrentPhase = ""
		if status.FailedLibraries > 0 {
			status.Status = "completed"
			status.Message = fmt.Sprintf("批量构建完成，%d 个资源库成功，%d 个失败", status.CompletedLibraries, status.FailedLibraries)
			return
		}
		status.Status = "completed"
		status.Message = fmt.Sprintf("批量构建完成，已处理 %d 个资源库", status.CompletedLibraries)
	})
	if snapshot := task.snapshot(); snapshot.ExitAfterComplete && m.shutdownAfterDone != nil {
		_ = m.shutdownAfterDone()
	}
}

func (m *libraryBatchBuildManager) failTask(task *libraryBatchBuildTask, err error) {
	message := "批量构建失败"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = err.Error()
	}
	task.mutate(func(status *api.LibraryBatchBuildStatus) {
		status.Status = "failed"
		status.Message = "批量构建失败"
		status.Error = message
		status.FinishedAt = time.Now().Format(time.RFC3339)
	})
}

func (m *libraryBatchBuildManager) beginLibrary(task *libraryBatchBuildTask, index int, library config.Library) {
	task.mutate(func(status *api.LibraryBatchBuildStatus) {
		status.Message = fmt.Sprintf("正在扫描资源库 %s", library.Name)
		status.CurrentLibraryIndex = index + 1
		status.CurrentLibraryName = library.Name
		status.CurrentLibraryPath = library.Path
		status.CurrentPhase = "scan"
		status.CurrentDone = 0
		status.CurrentTotal = 0
		status.CurrentPercent = 0
		status.Libraries[index].Status = "scanning"
		status.Libraries[index].Message = "正在扫描媒体"
	})
}

func (m *libraryBatchBuildManager) failLibrary(task *libraryBatchBuildTask, index int, err error) {
	task.mutate(func(status *api.LibraryBatchBuildStatus) {
		status.FailedLibraries++
		status.Libraries[index].Status = "failed"
		status.Libraries[index].Message = strings.TrimSpace(err.Error())
		status.Libraries[index].Failed++
		status.Message = fmt.Sprintf("资源库 %s 构建失败，继续处理下一个", status.Libraries[index].Name)
		status.CurrentPhase = ""
	})
}

func (m *libraryBatchBuildManager) cancelTask(task *libraryBatchBuildTask, currentIndex int) {
	task.mutate(func(status *api.LibraryBatchBuildStatus) {
		status.Status = "cancelled"
		status.Message = "已取消批量构建资源库"
		status.FinishedAt = time.Now().Format(time.RFC3339)
		status.CurrentPhase = ""
		if currentIndex >= 0 && currentIndex < len(status.Libraries) {
			if status.Libraries[currentIndex].Status == "scanning" || status.Libraries[currentIndex].Status == "building" {
				status.Libraries[currentIndex].Status = "cancelled"
				status.Libraries[currentIndex].Message = "已取消"
			}
		}
	})
}

func waitThumbnailBuildTask(ctx context.Context, svc *service.PhotoService, task *libraryBatchBuildTask, index int, library config.Library, userID int64) (service.ThumbnailBuildStatus, bool) {
	ticker := time.NewTicker(350 * time.Millisecond)
	defer ticker.Stop()

	for {
		status := svc.GetThumbnailBuildStatus(userID)
		task.mutate(func(batch *api.LibraryBatchBuildStatus) {
			batch.CurrentLibraryIndex = index + 1
			batch.CurrentLibraryName = library.Name
			batch.CurrentLibraryPath = library.Path
			batch.CurrentPhase = "thumbnails"
			batch.CurrentDone = status.Done
			batch.CurrentTotal = status.Total
			batch.CurrentPercent = status.Percent
			batch.Message = fmt.Sprintf("正在为资源库 %s 生成缩略图", library.Name)
			batch.Libraries[index].Generated = status.Generated
			batch.Libraries[index].Failed = status.Failed
			batch.Libraries[index].Status = "building"
			batch.Libraries[index].Message = status.Message
		})
		switch status.Status {
		case "completed", "failed", "cancelled":
			return status, false
		}
		select {
		case <-ctx.Done():
			_, _ = svc.CancelThumbnailBuild(userID)
			for {
				status = svc.GetThumbnailBuildStatus(userID)
				if status.Status != "running" && status.Status != "cancelling" {
					return status, true
				}
				time.Sleep(120 * time.Millisecond)
			}
		case <-ticker.C:
		}
	}
}

func (s *libraryBuildState) Status() api.LibraryBuildStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.status
	now := time.Now()
	var started time.Time
	if status.StartedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, status.StartedAt); err == nil {
			started = parsed
		}
	}
	if !started.IsZero() {
		end := now
		if status.FinishedAt != "" {
			if parsed, err := time.Parse(time.RFC3339, status.FinishedAt); err == nil {
				end = parsed
			}
		}
		status.ElapsedSeconds = int64(end.Sub(started).Seconds())
		if status.Done > 0 && status.Total > status.Done && status.Status == "running" {
			perItem := end.Sub(started).Seconds() / float64(status.Done)
			status.ETASeconds = int64(perItem * float64(status.Total-status.Done))
		}
	}
	if status.Total > 0 {
		status.Percent = float64(status.Done) / float64(status.Total) * 100
		if status.Percent > 100 {
			status.Percent = 100
		}
	}
	return status
}

func (s *libraryBuildState) SetExitAfterComplete(enabled bool) (api.LibraryBuildStatus, error) {
	s.mu.Lock()
	s.status.ExitAfterComplete = enabled
	status := s.status
	s.mu.Unlock()
	if enabled && (status.Status == "completed" || status.Status == "error") && s.shutdownAfterDone != nil {
		_ = s.shutdownAfterDone()
	}
	return s.Status(), nil
}

func (s *libraryBuildState) SetCancel(cancel context.CancelFunc) {
	s.mu.Lock()
	s.cancelBuild = cancel
	s.mu.Unlock()
}

func (s *libraryBuildState) Cancel() (api.LibraryBuildStatus, error) {
	s.mu.Lock()
	cancel := s.cancelBuild
	if s.status.Status == "discovering" || s.status.Status == "running" {
		s.status.Status = "cancelling"
		s.status.Message = "正在停止资源库构建"
		s.status.UpdatedAt = time.Now().Format(time.RFC3339)
	}
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return s.Status(), nil
}

func (s *libraryBuildState) clearCancel() {
	s.mu.Lock()
	s.cancelBuild = nil
	s.mu.Unlock()
}

func (s *libraryBuildState) start(message string) {
	now := time.Now().Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.Status = "discovering"
	s.status.Message = message
	s.status.StartedAt = now
	s.status.UpdatedAt = now
	s.status.FinishedAt = ""
	s.status.Done = 0
	s.status.Total = 0
	s.status.Imported = 0
	s.status.Skipped = 0
	s.status.Pruned = 0
	s.status.Error = ""
}

func (s *libraryBuildState) progress(done, total int) {
	now := time.Now().Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()
	if total > 0 {
		s.status.Status = "running"
		s.status.Message = "正在扫描与构建资源库"
	}
	s.status.Done = done
	s.status.Total = total
	s.status.UpdatedAt = now
}

func (s *libraryBuildState) finish(summary *service.ImportSummary, err error) {
	now := time.Now().Format(time.RFC3339)
	exitAfterComplete := false
	s.mu.Lock()
	if err != nil {
		s.status.Status = "error"
		s.status.Message = "资源库构建失败"
		s.status.Error = err.Error()
	} else {
		s.status.Status = "completed"
		s.status.Message = "资源库构建完成"
		if summary != nil {
			s.status.Imported = summary.Imported
			s.status.Skipped = summary.Skipped
			s.status.Pruned = summary.Pruned
		}
		if s.status.Total == 0 {
			s.status.Percent = 100
		}
	}
	s.status.UpdatedAt = now
	s.status.FinishedAt = now
	exitAfterComplete = s.status.ExitAfterComplete
	s.mu.Unlock()
	if exitAfterComplete && s.shutdownAfterDone != nil {
		_ = s.shutdownAfterDone()
	}
}

func (s *libraryBuildState) cancel() {
	now := time.Now().Format(time.RFC3339)
	s.mu.Lock()
	s.status.Status = "idle"
	s.status.Message = "资源库构建已取消"
	s.status.Error = ""
	s.status.UpdatedAt = now
	s.status.FinishedAt = now
	s.mu.Unlock()
}

func main() {
	if delay := strings.TrimSpace(os.Getenv(restartDelayEnv)); delay != "" {
		if ms, err := strconv.Atoi(delay); err == nil && ms > 0 {
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
	}

	// 处理子命令
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "adduser":
			if err := config.RunAddUserWizard(); err != nil {
				fmt.Fprintf(os.Stderr, "错误: %v\n", err)
				os.Exit(1)
			}
			return
		case "deluser":
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "用法: Echogallery deluser <username>")
				os.Exit(1)
			}
			if err := config.RunDeleteUserWizard(os.Args[2]); err != nil {
				fmt.Fprintf(os.Stderr, "错误: %v\n", err)
				os.Exit(1)
			}
			return
		default:
			fmt.Fprintf(os.Stderr, "未知命令: %s\n", os.Args[1])
			os.Exit(1)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		if errors.Is(err, config.ErrConfigNotFound) {
			startSetupServer(api.SetupState{
				Mode:    api.SetupModeInit,
				Message: "未找到 config.json，请先完成网页初始化。",
				Port:    8080,
			})
			return
		}
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("配置加载成功，服务将运行在端口 %d\n", cfg.Port)
	fmt.Printf("图片存储路径: %s\n", cfg.StoragePath)

	if strings.TrimSpace(cfg.StoragePath) == "" && len(cfg.Libraries) == 0 {
		startSetupServer(api.SetupState{
			Mode:    api.SetupModeInit,
			Message: "当前尚未配置资源库，请先完成网页初始化。",
			Port:    cfg.Port,
		})
		return
	}

	dbPath, err := cfg.DatabasePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 计算数据库路径失败: %v\n", err)
		os.Exit(1)
	}
	managedDataDir, err := cfg.ManagedDataDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 计算应用数据目录失败: %v\n", err)
		os.Exit(1)
	}
	trashDir, err := cfg.TrashPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 计算回收站目录失败: %v\n", err)
		os.Exit(1)
	}
	thumbDir, err := cfg.ThumbnailStoragePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 计算缩略图目录失败: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 创建数据库目录失败: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(managedDataDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 创建应用数据目录失败: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(trashDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 创建回收站目录失败: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(thumbDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 创建缩略图目录失败: %v\n", err)
		os.Exit(1)
	}
	repo, err := sqlite.New(dbPath)
	if err != nil {
		startSetupServer(api.SetupState{
			Mode:               api.SetupModeLibraryRecovery,
			Message:            "当前资源库无法打开数据库，请选择其他资源库后重启。",
			Libraries:          cfg.Libraries,
			CurrentStoragePath: cfg.StoragePath,
			Port:               cfg.Port,
		})
		return
	}
	defer repo.Close()

	if info, err := os.Stat(cfg.StoragePath); err != nil || !info.IsDir() {
		message := fmt.Sprintf("当前资源库不可用: %s", cfg.StoragePath)
		if err != nil {
			if os.IsPermission(err) || strings.Contains(strings.ToLower(err.Error()), "permission denied") {
				message = fmt.Sprintf("当前资源库无法访问，可能是权限不足: %v。请更换或新建资源库。", err)
			} else {
				message = fmt.Sprintf("当前资源库不可用: %v", err)
			}
		}
		startSetupServer(api.SetupState{
			Mode:               api.SetupModeLibraryRecovery,
			Message:            message,
			Libraries:          cfg.Libraries,
			CurrentStoragePath: cfg.StoragePath,
			Port:               cfg.Port,
		})
		return
	}
	if err := repo.CheckStoragePathConsistency(cfg.StoragePath, managedDataDir); err != nil {
		startSetupServer(api.SetupState{
			Mode:               api.SetupModeLibraryRecovery,
			Message:            fmt.Sprintf("当前资源库存在问题: %v", err),
			Libraries:          cfg.Libraries,
			CurrentStoragePath: cfg.StoragePath,
			Port:               cfg.Port,
		})
		return
	}

	service.SetLowResourceMode(cfg.Preferences.LowResourceMode)
	photoService := service.NewPhotoService(repo, cfg.StoragePath, managedDataDir, trashDir)
	photoService.SetThumbnailRoot(thumbDir)
	photoService.SetThumbnailSize(cfg.ThumbnailSize)
	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	rootCtx, cancelRoot := context.WithCancel(signalCtx)
	defer cancelRoot()
	shutdownCurrentProcess := makeShutdownCurrentProcess(cancelRoot)

	buildState := newLibraryBuildState(shutdownCurrentProcess)
	batchBuildManager := newLibraryBatchBuildManager(shutdownCurrentProcess)
	listener, actualPort, err := listenTCPWithFallback(cfg.Port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: HTTP 服务启动失败: %v\n", err)
		os.Exit(1)
	}
	if actualPort != cfg.Port {
		fmt.Printf("端口 %d 已被占用，已自动切换到 %d\n", cfg.Port, actualPort)
		cfg.Port = actualPort
	}

	app := api.NewRouterWithStaticWithLifecycleAndBuild(cfg, webFS, photoService, restartCurrentProcess, shutdownCurrentProcess, api.LibraryBuildHooks{
		Status:               buildState.Status,
		SetExitAfterComplete: buildState.SetExitAfterComplete,
		Cancel:               buildState.Cancel,
	}, api.LibraryBatchBuildHooks{
		Status:               batchBuildManager.Status,
		Start:                batchBuildManager.Start,
		Cancel:               batchBuildManager.Cancel,
		SetExitAfterComplete: batchBuildManager.SetExitAfterComplete,
	})
	addr := fmt.Sprintf(":%d", cfg.Port)
	if host := preferredLANIP(); host != "" {
		fmt.Printf("HTTP 服务已启动: http://%s%s\n", host, addr)
	} else {
		fmt.Printf("HTTP 服务已启动: http://127.0.0.1%s\n", addr)
	}
	if len(cfg.Users) > 0 {
		go runLibraryBuild(rootCtx, photoService, buildState)
	}
	if err := serveWithGracefulShutdown(rootCtx, listener, app); err != nil {
		fmt.Fprintf(os.Stderr, "错误: HTTP 服务启动失败: %v\n", err)
		os.Exit(1)
	}
}

func startSetupServer(state api.SetupState) {
	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	rootCtx, cancelRoot := context.WithCancel(signalCtx)
	defer cancelRoot()

	port := state.Port
	if port <= 0 {
		port = 8080
	}
	listener, actualPort, err := listenTCPWithFallback(port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 设置服务启动失败: %v\n", err)
		os.Exit(1)
	}
	if actualPort != port {
		fmt.Printf("设置端口 %d 已被占用，已自动切换到 %d\n", port, actualPort)
	}
	state.Port = actualPort
	addr := fmt.Sprintf(":%d", actualPort)
	app := api.NewSetupRouterWithStatic(webFS, state, restartCurrentProcess)
	fmt.Printf("EchoGallery 设置服务已启动: http://127.0.0.1%s\n", addr)
	if host := preferredLANIP(); host != "" {
		fmt.Printf("局域网访问地址: http://%s%s\n", host, addr)
	}
	if err := serveWithGracefulShutdown(rootCtx, listener, app); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 设置服务启动失败: %v\n", err)
		os.Exit(1)
	}
}

func runLibraryBuild(ctx context.Context, photoService *service.PhotoService, buildState *libraryBuildState) {
	buildCtx, cancelBuild := context.WithCancel(ctx)
	buildState.SetCancel(cancelBuild)
	defer buildState.clearCancel()
	defer cancelBuild()
	buildState.start("正在发现资源库中的媒体文件")
	summary, err := photoService.ImportExistingPhotosContext(buildCtx, 1, func(done, total int) {
		buildState.progress(done, total)
		if total == 0 {
			return
		}
		const width = 28
		filled := done * width / total
		if filled < 0 {
			filled = 0
		}
		if filled > width {
			filled = width
		}
		fmt.Printf("\r资源库扫描中 [%s%s] %d/%d", strings.Repeat("#", filled), strings.Repeat("-", width-filled), done, total)
		if done == total {
			fmt.Print("\n")
		}
	})
	if errors.Is(err, context.Canceled) {
		fmt.Println("\n资源库扫描已取消")
		buildState.cancel()
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 导入历史图片失败: %v\n", err)
	}
	if summary != nil && summary.Imported > 0 {
		fmt.Printf("已导入 %d 张历史图片\n", summary.Imported)
	}
	if summary != nil && summary.Pruned > 0 {
		fmt.Printf("已清理 %d 条失效媒体记录\n", summary.Pruned)
	}
	buildState.finish(summary, err)
}

func serveWithGracefulShutdown(ctx context.Context, listener net.Listener, handler http.Handler) error {
	server := &http.Server{Handler: handler}
	errCh := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return shutdownHTTPServer(server, errCh)
	case <-shutdownRequested:
		return shutdownHTTPServer(server, errCh)
	}
}

func shutdownHTTPServer(server *http.Server, errCh <-chan error) error {
	fmt.Println("\n正在关闭 EchoGallery...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return err
	}
	return <-errCh
}

func listenTCPWithFallback(preferredPort int) (net.Listener, int, error) {
	if preferredPort <= 0 {
		preferredPort = 8080
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", preferredPort))
	if err == nil {
		return listener, preferredPort, nil
	}
	if !errors.Is(err, syscall.EADDRINUSE) {
		return nil, 0, err
	}

	for port := preferredPort + 1; port <= preferredPort+portFallbackScanLimit; port++ {
		listener, err = net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			return listener, port, nil
		}
		if !errors.Is(err, syscall.EADDRINUSE) {
			return nil, 0, err
		}
	}

	listener, err = net.Listen("tcp", ":0")
	if err != nil {
		return nil, 0, err
	}
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		listener.Close()
		return nil, 0, fmt.Errorf("无法解析监听端口: %T", listener.Addr())
	}
	return listener, addr.Port, nil
}

func restartCurrentProcess() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	wd, _ := os.Getwd()
	go func() {
		time.Sleep(150 * time.Millisecond)
		if wd != "" {
			_ = os.Chdir(wd)
		}
		env := append(os.Environ(), restartDelayEnv+"=1200")
		args := append([]string{exe}, os.Args[1:]...)
		if runtime.GOOS == "windows" {
			cmd := exec.Command(exe, os.Args[1:]...)
			cmd.Dir = wd
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Env = env
			if err := cmd.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "错误: 重启 EchoGallery 失败: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		}
		if err := syscall.Exec(exe, args, env); err != nil {
			fmt.Fprintf(os.Stderr, "错误: 重启 EchoGallery 失败: %v\n", err)
			os.Exit(1)
		}
	}()
	return nil
}

func makeShutdownCurrentProcess(cancel context.CancelFunc) func() error {
	return func() error {
		go func() {
			time.Sleep(150 * time.Millisecond)
			cancel()
			select {
			case shutdownRequested <- struct{}{}:
			default:
			}
		}()
		return nil
	}
}

func preferredLANIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}

	var fallback string
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP == nil || ipNet.IP.IsLoopback() {
			continue
		}

		ip := ipNet.IP.To4()
		if ip == nil || !isPrivateIPv4(ip) {
			continue
		}

		if ip[0] == 192 && ip[1] == 168 {
			return ip.String()
		}
		if fallback == "" {
			fallback = ip.String()
		}
	}

	return fallback
}

func isPrivateIPv4(ip net.IP) bool {
	return ip[0] == 10 ||
		(ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31) ||
		(ip[0] == 192 && ip[1] == 168)
}
