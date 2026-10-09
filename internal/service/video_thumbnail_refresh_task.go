package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"echogallery/internal/storage"
)

type VideoThumbnailRefreshStatus struct {
	Status         string
	Message        string
	Done           int
	Total          int
	Refreshed      int
	Failed         int
	StartedAt      time.Time
	UpdatedAt      time.Time
	FinishedAt     time.Time
	ElapsedSeconds int64
	ETASeconds     int64
	Percent        float64
	Error          string
}

type videoThumbnailRefreshTask struct {
	userID int64
	cancel context.CancelFunc

	mu     sync.Mutex
	status VideoThumbnailRefreshStatus
}

func (s *PhotoService) StartVideoThumbnailRefresh(userID int64) (VideoThumbnailRefreshStatus, error) {
	if s == nil {
		return VideoThumbnailRefreshStatus{}, fmt.Errorf("媒体服务不可用")
	}
	s.thumbBuildMu.Lock()
	defer s.thumbBuildMu.Unlock()

	if s.videoThumbRefresh != nil {
		current := s.videoThumbRefresh.snapshot()
		if current.Status == "running" || current.Status == "cancelling" {
			return current, nil
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	task := &videoThumbnailRefreshTask{
		userID: userID,
		cancel: cancel,
		status: VideoThumbnailRefreshStatus{
			Status:    "running",
			Message:   "正在扫描视频并刷新缩略图…",
			StartedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
	s.videoThumbRefresh = task
	s.background(func() { s.runVideoThumbnailRefresh(ctx, task) })
	return task.snapshot(), nil
}

func (s *PhotoService) GetVideoThumbnailRefreshStatus(userID int64) VideoThumbnailRefreshStatus {
	if s == nil {
		return VideoThumbnailRefreshStatus{Status: "idle", Message: "当前没有视频缩略图任务"}
	}
	s.thumbBuildMu.Lock()
	task := s.videoThumbRefresh
	s.thumbBuildMu.Unlock()
	if task == nil || task.userID != userID {
		return VideoThumbnailRefreshStatus{Status: "idle", Message: "当前没有视频缩略图任务"}
	}
	return task.snapshot()
}

func (s *PhotoService) CancelVideoThumbnailRefresh(userID int64) (VideoThumbnailRefreshStatus, error) {
	if s == nil {
		return VideoThumbnailRefreshStatus{Status: "idle", Message: "当前没有视频缩略图任务"}, nil
	}
	s.thumbBuildMu.Lock()
	task := s.videoThumbRefresh
	s.thumbBuildMu.Unlock()
	if task == nil || task.userID != userID {
		return VideoThumbnailRefreshStatus{Status: "idle", Message: "当前没有视频缩略图任务"}, nil
	}
	task.mu.Lock()
	if task.status.Status == "running" {
		task.status.Status = "cancelling"
		task.status.Message = "正在取消视频缩略图刷新…"
		task.status.UpdatedAt = time.Now()
	}
	cancel := task.cancel
	task.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return task.snapshot(), nil
}

func (s *PhotoService) runVideoThumbnailRefresh(ctx context.Context, task *videoThumbnailRefreshTask) {
	videos := make([]*storage.Photo, 0, 256)
	for _, trashed := range []bool{false, true} {
		cursor := ""
		for {
			if err := ctx.Err(); err != nil {
				task.finish("cancelled", "已取消视频缩略图刷新", "")
				return
			}
			params := storage.ListPhotosParams{UserID: task.userID, Cursor: cursor, Limit: 200}
			var page *storage.PhotoPage
			var err error
			if trashed {
				page, err = s.repo.ListTrashedPhotos(params)
			} else {
				page, err = s.repo.ListPhotos(params)
			}
			if err != nil {
				task.finish("failed", "视频缩略图刷新失败", err.Error())
				return
			}
			for _, photo := range page.Photos {
				if photo.MediaKind == storage.MediaKindVideo {
					videos = append(videos, photo)
				}
			}
			if !page.HasMore || page.NextCursor == "" {
				break
			}
			cursor = page.NextCursor
		}
	}

	task.setTotal(len(videos))
	if len(videos) == 0 {
		task.finish("completed", "没有需要刷新的视频", "")
		return
	}

	for _, photo := range videos {
		if err := ctx.Err(); err != nil {
			task.finish("cancelled", "已取消视频缩略图刷新", "")
			return
		}
		if err := s.generateThumbnailForPhoto(photo); err != nil {
			task.incrementFailed()
			continue
		}
		task.incrementRefreshed()
	}

	if err := ctx.Err(); err != nil {
		task.finish("cancelled", "已取消视频缩略图刷新", "")
		return
	}
	task.finish("completed", "视频缩略图刷新完成", "")
}

func (t *videoThumbnailRefreshTask) setTotal(total int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.Total = total
	t.status.UpdatedAt = time.Now()
}

func (t *videoThumbnailRefreshTask) incrementRefreshed() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.Refreshed++
	t.status.Done++
	t.status.Message = fmt.Sprintf("正在刷新视频缩略图… 已完成 %d 项", t.status.Done)
	t.status.UpdatedAt = time.Now()
}

func (t *videoThumbnailRefreshTask) incrementFailed() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.Failed++
	t.status.Done++
	t.status.Message = fmt.Sprintf("正在刷新视频缩略图… 已完成 %d 项", t.status.Done)
	t.status.UpdatedAt = time.Now()
}

func (t *videoThumbnailRefreshTask) finish(status, message, errText string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	t.status.Status = status
	t.status.Message = message
	t.status.Error = errText
	t.status.UpdatedAt = now
	t.status.FinishedAt = now
}

func (t *videoThumbnailRefreshTask) snapshot() VideoThumbnailRefreshStatus {
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
