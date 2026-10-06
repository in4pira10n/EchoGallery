package service

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
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

type thumbnailBuildJob struct {
	photo *storage.Photo
	tier  string
}

func imageThumbnailBuildWorkerCount() int {
	if lowResourceModeEnabled() {
		return 1
	}
	if batchAggressiveModeEnabled() {
		n := runtime.GOMAXPROCS(0) / 3
		if n < 2 {
			return 2
		}
		if n > 6 {
			return 6
		}
		return n
	}
	n := runtime.GOMAXPROCS(0) / 5
	if n < 1 {
		return 1
	}
	if n > 3 {
		return 3
	}
	return n
}

func heavyImageThumbnailBuildWorkerCount() int {
	if batchAggressiveModeEnabled() && !lowResourceModeEnabled() {
		return 2
	}
	return 1
}

func videoThumbnailBuildWorkerCount() int {
	if lowResourceModeEnabled() {
		return 1
	}
	if batchAggressiveModeEnabled() {
		n := runtime.GOMAXPROCS(0) / 5
		if n < 2 {
			return 2
		}
		if n > 4 {
			return 4
		}
		return n
	}
	n := runtime.GOMAXPROCS(0) / 8
	if n < 1 {
		return 1
	}
	if n > 2 {
		return 2
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
			Message:   thumbnailBuildStartMessage(),
			StartedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
	s.thumbBuild = task
	s.thumbnailBuildActive.Store(true)
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
	defer s.thumbnailBuildActive.Store(false)
	heavyImageJobs := make(chan thumbnailBuildJob, heavyImageThumbnailBuildWorkerCount()*4)
	imageJobs := make(chan thumbnailBuildJob, imageThumbnailBuildWorkerCount()*6)
	videoJobs := make(chan thumbnailBuildJob, videoThumbnailBuildWorkerCount()*6)
	var workerWG sync.WaitGroup
	for i := 0; i < heavyImageThumbnailBuildWorkerCount(); i++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			s.runThumbnailBuildWorker(ctx, task, heavyImageJobs)
		}()
	}
	for i := 0; i < imageThumbnailBuildWorkerCount(); i++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			s.runThumbnailBuildWorker(ctx, task, imageJobs)
		}()
	}
	for i := 0; i < videoThumbnailBuildWorkerCount(); i++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			s.runThumbnailBuildWorker(ctx, task, videoJobs)
		}()
	}

	cursor := ""
	firstPage := true
	pageLimit := 320
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
		sortPhotosForSequentialThumbnailBuild(page.Photos)
		targetTier := thumbnailTierFull
		targetLabel := thumbnailBuildTierLabel(targetTier)
		for _, photo := range page.Photos {
			if err := ctx.Err(); err != nil {
				break
			}
			missingTarget := !s.thumbnailTargetExists(photo)
			if !missingTarget {
				task.incrementSkipped(targetLabel)
			}
			if !missingTarget {
				continue
			}
			if !enqueueThumbnailBuildJob(ctx, task, heavyImageJobs, imageJobs, videoJobs, thumbnailBuildJob{photo: photo, tier: targetTier}) {
				close(heavyImageJobs)
				close(imageJobs)
				close(videoJobs)
				workerWG.Wait()
				task.finish("cancelled", "已取消缩略图生成", "")
				return
			}
		}
		if ctx.Err() != nil || !page.HasMore || page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
		task.setMessage(thumbnailBuildQueueMessage())
	}

	close(heavyImageJobs)
	close(imageJobs)
	close(videoJobs)
	workerWG.Wait()

	if ctx.Err() != nil {
		task.finish("cancelled", thumbnailBuildCancelledMessage(), "")
		return
	}
	if runErr != nil {
		task.finish("failed", thumbnailBuildFailedMessage(), runErr.Error())
		return
	}
	task.finish("completed", thumbnailBuildCompletedMessage(), "")
}

func thumbnailBuildTierLabel(tier string) string {
	switch tier {
	case thumbnailTierFull:
		return "标准缩略图"
	default:
		return "缩略图"
	}
}

func thumbnailBuildStartMessage() string {
	return "正在扫描媒体并生成标准缩略图…"
}

func thumbnailBuildQueueMessage() string {
	return "正在排队标准缩略图任务…"
}

func thumbnailBuildCompletedMessage() string {
	return "标准缩略图已生成完成"
}

func thumbnailBuildCancelledMessage() string {
	return "已取消标准缩略图生成"
}

func thumbnailBuildFailedMessage() string {
	return "标准缩略图生成失败"
}

func enqueueThumbnailBuildJob(ctx context.Context, task *thumbnailBuildTask, heavyImageJobs chan<- thumbnailBuildJob, imageJobs chan<- thumbnailBuildJob, videoJobs chan<- thumbnailBuildJob, job thumbnailBuildJob) bool {
	target := imageJobs
	if job.photo != nil && job.photo.MediaKind == storage.MediaKindVideo {
		target = videoJobs
	} else if isMemoryHeavyThumbnail(job.photo) {
		target = heavyImageJobs
	}
	select {
	case <-ctx.Done():
		return false
	case target <- job:
		task.setMessage(formatThumbnailBuildQueueMessage(job))
		return true
	}
}

func formatThumbnailBuildQueueMessage(job thumbnailBuildJob) string {
	if job.photo == nil {
		return "正在生成缩略图…"
	}
	kind := "图片"
	if job.photo.MediaKind == storage.MediaKindVideo {
		kind = "视频"
	}
	label := thumbnailBuildTierLabel(job.tier)
	return fmt.Sprintf("正在生成%s%s…", kind, label)
}

func sortPhotosForSequentialThumbnailBuild(photos []*storage.Photo) {
	sort.SliceStable(photos, func(i, j int) bool {
		pi := photos[i]
		pj := photos[j]
		if pi == nil || pj == nil {
			return pi != nil
		}
		di := thumbnailBuildDirectoryKey(pi)
		dj := thumbnailBuildDirectoryKey(pj)
		if di != dj {
			return di < dj
		}
		if pi.MediaKind != pj.MediaKind {
			return pi.MediaKind < pj.MediaKind
		}
		return pi.SourceRelPath < pj.SourceRelPath
	})
}

func thumbnailBuildDirectoryKey(photo *storage.Photo) string {
	if photo == nil {
		return ""
	}
	source := filepath.Clean(strings.TrimSpace(photo.SourceRelPath))
	if source == "." || source == "" {
		return ""
	}
	return filepath.Dir(source)
}

func (s *PhotoService) runThumbnailBuildWorker(ctx context.Context, task *thumbnailBuildTask, jobs <-chan thumbnailBuildJob) {
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-jobs:
			if !ok {
				return
			}
			if job.photo == nil {
				continue
			}
			if err := s.generateThumbnailForPhotoTier(job.photo, job.tier); err != nil {
				label := thumbnailBuildTierLabel(job.tier)
				task.incrementFailed(label)
				continue
			}
			label := thumbnailBuildTierLabel(job.tier)
			task.incrementGenerated(label)
		}
	}
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

func (t *thumbnailBuildTask) incrementGenerated(label string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.Generated++
	t.status.Done++
	if label == "" {
		label = "缩略图"
	}
	t.status.Message = fmt.Sprintf("正在生成%s… 已完成 %d 项", label, t.status.Done)
	t.status.UpdatedAt = time.Now()
}

func (t *thumbnailBuildTask) incrementSkipped(label string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.Skipped++
	t.status.Done++
	if label == "" {
		label = "缩略图"
	}
	t.status.Message = fmt.Sprintf("正在检查%s… 已完成 %d 项", label, t.status.Done)
	t.status.UpdatedAt = time.Now()
}

func (t *thumbnailBuildTask) incrementFailed(label string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.Failed++
	t.status.Done++
	if label == "" {
		label = "缩略图"
	}
	t.status.Message = fmt.Sprintf("%s生成失败… 已完成 %d 项", label, t.status.Done)
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
