package service

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"echogallery/internal/storage"
)

type ThumbnailBuildStatus struct {
	Status         string
	Message        string
	Done           int
	Total          int
	Generated      int
	Skipped        int
	Failed         int
	StartedAt      time.Time
	UpdatedAt      time.Time
	FinishedAt     time.Time
	ElapsedSeconds int64
	ETASeconds     int64
	Percent        float64
	Error          string
}

type thumbnailBuildTask struct {
	userID int64
	cancel context.CancelFunc

	mu     sync.Mutex
	status ThumbnailBuildStatus
}

func thumbnailBuildWorkerCount() int {
	base := runtime.GOMAXPROCS(0)
	n := base + (base / 2)
	if n < 4 {
		return 4
	}
	if n > 16 {
		return 16
	}
	return n
}

func (s *PhotoService) StartThumbnailBuild(userID int64) (ThumbnailBuildStatus, error) {
	if s == nil {
		return ThumbnailBuildStatus{}, fmt.Errorf("媒体服务不可用")
	}
	s.thumbBuildMu.Lock()
	defer s.thumbBuildMu.Unlock()

	if s.thumbBuild != nil {
		current := s.thumbBuild.snapshot()
		if current.Status == "running" || current.Status == "cancelling" {
			return current, nil
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	task := &thumbnailBuildTask{
		userID: userID,
		cancel: cancel,
		status: ThumbnailBuildStatus{
			Status:    "running",
			Message:   "正在扫描媒体并生成缩略图…",
			StartedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
	s.thumbBuild = task
	go s.runThumbnailBuild(ctx, task)
	return task.snapshot(), nil
}

func (s *PhotoService) GetThumbnailBuildStatus(userID int64) ThumbnailBuildStatus {
	if s == nil {
		return ThumbnailBuildStatus{Status: "idle", Message: "当前没有缩略图任务"}
	}
	s.thumbBuildMu.Lock()
	task := s.thumbBuild
	s.thumbBuildMu.Unlock()
	if task == nil || task.userID != userID {
		return ThumbnailBuildStatus{Status: "idle", Message: "当前没有缩略图任务"}
	}
	return task.snapshot()
}

func (s *PhotoService) CancelThumbnailBuild(userID int64) (ThumbnailBuildStatus, error) {
	if s == nil {
		return ThumbnailBuildStatus{Status: "idle", Message: "当前没有缩略图任务"}, nil
	}
	s.thumbBuildMu.Lock()
	task := s.thumbBuild
	s.thumbBuildMu.Unlock()
	if task == nil || task.userID != userID {
		return ThumbnailBuildStatus{Status: "idle", Message: "当前没有缩略图任务"}, nil
	}
	task.mu.Lock()
	if task.status.Status == "running" {
		task.status.Status = "cancelling"
		task.status.Message = "正在取消缩略图生成…"
		task.status.UpdatedAt = time.Now()
	}
	cancel := task.cancel
	task.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return task.snapshot(), nil
}

func (s *PhotoService) runThumbnailBuild(ctx context.Context, task *thumbnailBuildTask) {
	jobs := make(chan *storage.Photo, thumbnailBuildWorkerCount()*2)
	var workerWG sync.WaitGroup
	for i := 0; i < thumbnailBuildWorkerCount(); i++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case photo, ok := <-jobs:
					if !ok {
						return
					}
					if photo == nil {
						continue
					}
					if err := s.generateThumbnailForPhoto(photo); err != nil {
						task.incrementFailed()
						continue
					}
					task.incrementGenerated()
				}
			}
		}()
	}

	cursor := ""
	firstPage := true
	pageLimit := 640
	var runErr error

	for {
		if err := ctx.Err(); err != nil {
			break
		}
		page, err := s.repo.ListPhotos(storage.ListPhotosParams{
			UserID:    task.userID,
			Cursor:    cursor,
			Limit:     pageLimit,
			SkipTotal: !firstPage,
		})
		if err != nil {
			runErr = err
			break
		}
		if page == nil {
			break
		}
		rawCount := len(page.Photos)
		page = s.filterExistingMediaPage(page)
		if firstPage {
			task.setTotal(page.Total)
			firstPage = false
		} else if removed := rawCount - len(page.Photos); removed > 0 {
			task.adjustTotal(-removed)
		}
		for _, photo := range page.Photos {
			if err := ctx.Err(); err != nil {
				break
			}
			if s.thumbnailAlreadyExists(photo) {
				task.incrementSkipped()
				continue
			}
			select {
			case <-ctx.Done():
				close(jobs)
				workerWG.Wait()
				task.finish("cancelled", "已取消缩略图生成", "")
				return
			case jobs <- photo:
			}
		}
		if ctx.Err() != nil || !page.HasMore || page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
		task.setMessage("正在扫描媒体并生成缩略图…")
	}

	close(jobs)
	workerWG.Wait()

	if ctx.Err() != nil {
		task.finish("cancelled", "已取消缩略图生成", "")
		return
	}
	if runErr != nil {
		task.finish("failed", "缩略图生成失败", runErr.Error())
		return
	}
	task.finish("completed", "全部缩略图已生成完成", "")
}

func (t *thumbnailBuildTask) setTotal(total int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if total > 0 {
		t.status.Total = total
	}
	t.status.UpdatedAt = time.Now()
}

func (t *thumbnailBuildTask) adjustTotal(delta int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.Total += delta
	if t.status.Total < 0 {
		t.status.Total = 0
	}
	t.status.UpdatedAt = time.Now()
}

func (t *thumbnailBuildTask) setMessage(message string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.Message = message
	t.status.UpdatedAt = time.Now()
}

func (t *thumbnailBuildTask) incrementGenerated() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.Generated++
	t.status.Done++
	t.status.Message = fmt.Sprintf("正在生成缩略图… 已完成 %d 项", t.status.Done)
	t.status.UpdatedAt = time.Now()
}

func (t *thumbnailBuildTask) incrementSkipped() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.Skipped++
	t.status.Done++
	t.status.Message = fmt.Sprintf("正在检查缩略图… 已完成 %d 项", t.status.Done)
	t.status.UpdatedAt = time.Now()
}

func (t *thumbnailBuildTask) incrementFailed() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.Failed++
	t.status.Done++
	t.status.Message = fmt.Sprintf("正在生成缩略图… 已完成 %d 项", t.status.Done)
	t.status.UpdatedAt = time.Now()
}

func (t *thumbnailBuildTask) finish(status, message, errText string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	t.status.Status = status
	t.status.Message = message
	t.status.Error = errText
	t.status.UpdatedAt = now
	t.status.FinishedAt = now
}

func (t *thumbnailBuildTask) snapshot() ThumbnailBuildStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	snapshot := t.status
	now := time.Now()
	if snapshot.StartedAt.IsZero() {
		return snapshot
	}
	end := now
	if !snapshot.FinishedAt.IsZero() {
		end = snapshot.FinishedAt
	}
	snapshot.ElapsedSeconds = int64(end.Sub(snapshot.StartedAt).Seconds())
	if snapshot.Total > 0 {
		snapshot.Percent = float64(snapshot.Done) / float64(snapshot.Total) * 100
		if snapshot.Percent > 100 {
			snapshot.Percent = 100
		}
	}
	if snapshot.Status == "running" || snapshot.Status == "cancelling" {
		if snapshot.Done > 0 && snapshot.Total > snapshot.Done && snapshot.ElapsedSeconds > 0 {
			perItem := float64(snapshot.ElapsedSeconds) / float64(snapshot.Done)
			snapshot.ETASeconds = int64(perItem * float64(snapshot.Total-snapshot.Done))
		}
	}
	return snapshot
}
