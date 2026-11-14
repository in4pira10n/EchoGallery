package service

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"sync"
	"time"

	imgpkg "echogallery/internal/image"
	"echogallery/internal/media"
	"echogallery/internal/storage"
)

// EXIFBackfillResult 批量修正媒体 EXIF / 拍摄时间信息的结果。
type EXIFBackfillResult struct {
	Scanned int      `json:"scanned"`
	Updated int      `json:"updated"`
	Skipped int      `json:"skipped"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors,omitempty"`
}

type EXIFBackfillStatus struct {
	Status         string    `json:"status"`
	Message        string    `json:"message"`
	CurrentMedia   string    `json:"current_media,omitempty"`
	Done           int       `json:"done"`
	Total          int       `json:"total"`
	Scanned        int       `json:"scanned"`
	Updated        int       `json:"updated"`
	Skipped        int       `json:"skipped"`
	Failed         int       `json:"failed"`
	Errors         []string  `json:"errors,omitempty"`
	StartedAt      time.Time `json:"started_at,omitempty"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
	FinishedAt     time.Time `json:"finished_at,omitempty"`
	ElapsedSeconds int64     `json:"elapsed_seconds"`
	ETASeconds     int64     `json:"eta_seconds"`
	Percent        float64   `json:"percent"`
	Error          string    `json:"error,omitempty"`
}

type exifBackfillTask struct {
	userID int64
	cancel context.CancelFunc

	mu     sync.Mutex
	status EXIFBackfillStatus
}

// BackfillPhotoEXIF 修正旧媒体的 EXIF、拍摄时间与基础元数据。
func (s *PhotoService) BackfillPhotoEXIF(userID int64) (*EXIFBackfillResult, error) {
	return s.BackfillPhotoEXIFContext(context.Background(), userID, nil)
}

// BackfillPhotoEXIFContext refreshes captured metadata while allowing a build
// workflow to cancel the pass and report progress.
func (s *PhotoService) BackfillPhotoEXIFContext(ctx context.Context, userID int64, progress func(done, total int)) (*EXIFBackfillResult, error) {
	result := &EXIFBackfillResult{}
	err := s.backfillPhotoEXIFContext(ctx, userID, func(status EXIFBackfillStatus) bool {
		result.Scanned = status.Scanned
		result.Updated = status.Updated
		result.Skipped = status.Skipped
		result.Failed = status.Failed
		result.Errors = append([]string(nil), status.Errors...)
		if progress != nil {
			progress(status.Done, status.Total)
		}
		return true
	})
	return result, err
}

func (s *PhotoService) StartEXIFBackfill(userID int64) (EXIFBackfillStatus, error) {
	if s == nil {
		return EXIFBackfillStatus{}, fmt.Errorf("媒体服务不可用")
	}
	s.exifBackfillMu.Lock()
	defer s.exifBackfillMu.Unlock()
	if s.exifBackfill != nil {
		current := s.exifBackfill.snapshot()
		if current.Status == "running" || current.Status == "cancelling" {
			return current, nil
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	task := &exifBackfillTask{
		userID: userID,
		cancel: cancel,
		status: EXIFBackfillStatus{
			Status:    "running",
			Message:   "正在准备修正 EXIF 信息…",
			StartedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
	s.exifBackfill = task
	if !s.background(func() { s.runEXIFBackfill(ctx, task) }) {
		cancel()
		task.finish("failed", "EXIF 信息修正失败", "service is shutting down")
		return task.snapshot(), fmt.Errorf("媒体服务正在关闭")
	}
	return task.snapshot(), nil
}

func (s *PhotoService) GetEXIFBackfillStatus(userID int64) EXIFBackfillStatus {
	if s == nil {
		return EXIFBackfillStatus{Status: "idle", Message: "当前没有 EXIF 修正任务"}
	}
	s.exifBackfillMu.Lock()
	task := s.exifBackfill
	s.exifBackfillMu.Unlock()
	if task == nil || task.userID != userID {
		return EXIFBackfillStatus{Status: "idle", Message: "当前没有 EXIF 修正任务"}
	}
	return task.snapshot()
}

func (s *PhotoService) CancelEXIFBackfill(userID int64) (EXIFBackfillStatus, error) {
	if s == nil {
		return EXIFBackfillStatus{Status: "idle", Message: "当前没有 EXIF 修正任务"}, nil
	}
	s.exifBackfillMu.Lock()
	task := s.exifBackfill
	s.exifBackfillMu.Unlock()
	if task == nil || task.userID != userID {
		return EXIFBackfillStatus{Status: "idle", Message: "当前没有 EXIF 修正任务"}, nil
	}
	task.mu.Lock()
	if task.status.Status == "running" {
		task.status.Status = "cancelling"
		task.status.Message = "正在取消 EXIF 信息修正…"
		task.status.UpdatedAt = time.Now()
	}
	cancel := task.cancel
	task.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return task.snapshot(), nil
}

func (s *PhotoService) runEXIFBackfill(ctx context.Context, task *exifBackfillTask) {
	err := s.backfillPhotoEXIFContext(ctx, task.userID, func(status EXIFBackfillStatus) bool {
		task.update(func(current *EXIFBackfillStatus) {
			current.Total = status.Total
			current.Done = status.Done
			current.CurrentMedia = status.CurrentMedia
			current.Scanned = status.Scanned
			current.Updated = status.Updated
			current.Skipped = status.Skipped
			current.Failed = status.Failed
			current.Errors = append([]string(nil), status.Errors...)
			current.Message = status.Message
			current.UpdatedAt = time.Now()
		})
		return true
	})
	if err != nil {
		if ctx.Err() != nil {
			task.finish("cancelled", "已取消 EXIF 信息修正", "")
			return
		}
		task.finish("failed", "EXIF 信息修正失败", err.Error())
		return
	}
	if ctx.Err() != nil {
		task.finish("cancelled", "已取消 EXIF 信息修正", "")
		return
	}
	task.finish("completed", "EXIF 信息修正完成", "")
}

func (s *PhotoService) backfillPhotoEXIFContext(ctx context.Context, userID int64, progress func(EXIFBackfillStatus) bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	status := EXIFBackfillStatus{
		Status:    "running",
		Message:   "正在读取媒体列表…",
		StartedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if progress != nil && !progress(status) {
		return context.Canceled
	}
	cursor := ""
	afterID := int64(0)
	firstPage := true
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		params := storage.ListPhotosParams{
			UserID:    userID,
			Cursor:    cursor,
			Limit:     200,
			SkipTotal: !firstPage,
		}
		page, idPaging, err := listPhotosForEXIFBackfill(ctx, s.repo, params, afterID, firstPage)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		firstPage = false
		if page.Total > 0 {
			status.Total = page.Total
		}
		status.Message = "正在扫描媒体…"
		status.UpdatedAt = time.Now()
		if progress != nil && !progress(status) {
			return context.Canceled
		}
		status.Scanned += page.Skipped
		status.Done += page.Skipped
		status.Skipped += page.Skipped
		if page.Skipped > 0 {
			status.UpdatedAt = time.Now()
			if progress != nil && !progress(status) {
				return context.Canceled
			}
		}
		for _, photo := range page.Photos {
			if err := ctx.Err(); err != nil {
				return err
			}
			status.CurrentMedia = ""
			if photo != nil {
				status.CurrentMedia = photo.OriginalName
			}
			status.Message = "正在扫描媒体…"
			status.UpdatedAt = time.Now()
			if progress != nil && !progress(status) {
				return context.Canceled
			}
			updated, skipped, err := s.repairMediaMetadata(ctx, photo, userID)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			status.Scanned++
			status.Done++
			if err != nil {
				status.Failed++
				if len(status.Errors) < 8 {
					name := ""
					if photo != nil {
						name = photo.OriginalName
					}
					if name == "" {
						name = "unknown media"
					}
					status.Errors = append(status.Errors, fmt.Sprintf("%s: %v", name, err))
				}
			} else if skipped {
				status.Skipped++
			} else if updated {
				status.Updated++
			}
			status.Message = fmt.Sprintf("正在修正 EXIF 信息… 已处理 %d 项", status.Done)
			status.CurrentMedia = ""
			status.UpdatedAt = time.Now()
			if progress != nil && !progress(status) {
				return context.Canceled
			}
		}
		if idPaging {
			if !page.HasMore {
				break
			}
			nextID := page.LastID
			if nextID == 0 && len(page.Photos) > 0 {
				nextID = page.Photos[len(page.Photos)-1].ID
			}
			if nextID <= afterID {
				return fmt.Errorf("metadata repair cursor did not advance")
			}
			afterID = nextID
		} else {
			if !page.HasMore || page.NextCursor == "" {
				break
			}
			cursor = page.NextCursor
		}
	}
	return nil
}

type exifBackfillPhotoLister interface {
	ListPhotosForMetadataBackfillContext(context.Context, int64, int64, int, bool) (*storage.PhotoPage, error)
}

func listPhotosForEXIFBackfill(ctx context.Context, repo storage.Repository, params storage.ListPhotosParams, afterID int64, includeTotal bool) (*storage.PhotoPage, bool, error) {
	if lister, ok := repo.(exifBackfillPhotoLister); ok {
		page, err := lister.ListPhotosForMetadataBackfillContext(ctx, params.UserID, afterID, params.Limit, includeTotal)
		return page, true, err
	}
	page, err := repo.ListPhotos(params)
	return page, false, err
}

func (s *PhotoService) repairMediaMetadata(ctx context.Context, photo *storage.Photo, userID int64) (updated bool, skipped bool, err error) {
	if err := ctx.Err(); err != nil {
		return false, false, err
	}
	if photo == nil {
		return false, true, nil
	}
	path := s.MediaPath(photo)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false, true, nil
	}
	switch photo.MediaKind {
	case storage.MediaKindVideo:
		return s.repairVideoMetadata(ctx, photo, userID, path, info.ModTime())
	default:
		return s.repairImageMetadata(ctx, photo, userID, path, info.ModTime())
	}
}

func (s *PhotoService) repairImageMetadata(ctx context.Context, photo *storage.Photo, userID int64, path string, fallbackTime time.Time) (bool, bool, error) {
	meta, err := imgpkg.ExtractMetaFromPathContext(ctx, path, photo.OriginalName, fallbackTime)
	if err != nil {
		return false, false, err
	}
	if meta == nil {
		return false, true, nil
	}
	if err := ctx.Err(); err != nil {
		return false, false, err
	}
	nextExif := copyPhotoEXIF(meta.EXIF)
	nextTakenAt := meta.TakenAt
	if imageMetadataUnchanged(photo, nextTakenAt, nextExif, meta.Width, meta.Height) {
		return false, true, nil
	}
	if err := updateCapturedMetadata(ctx, s.repo, photo.ID, userID, nextTakenAt, nextExif, meta.Width, meta.Height, photo.DurationMS); err != nil {
		return false, false, err
	}
	photo.TakenAt = nextTakenAt
	photo.EXIF = nextExif
	photo.Width = meta.Width
	photo.Height = meta.Height
	return true, false, nil
}

func (s *PhotoService) repairVideoMetadata(ctx context.Context, photo *storage.Photo, userID int64, path string, fallbackTime time.Time) (bool, bool, error) {
	meta, err := media.ProbeVideoContext(ctx, path)
	if err != nil {
		return false, false, err
	}
	if meta == nil {
		return false, true, nil
	}
	if err := ctx.Err(); err != nil {
		return false, false, err
	}
	nextTakenAt := fallbackTime
	if !meta.TakenAt.IsZero() {
		nextTakenAt = meta.TakenAt
	}
	nextExif := videoMetaEXIF(meta)
	if videoMetadataUnchanged(photo, nextTakenAt, nextExif, meta.Width, meta.Height, meta.DurationMS) {
		return false, true, nil
	}
	if err := updateCapturedMetadata(ctx, s.repo, photo.ID, userID, nextTakenAt, nextExif, meta.Width, meta.Height, meta.DurationMS); err != nil {
		return false, false, err
	}
	photo.TakenAt = nextTakenAt
	photo.EXIF = nextExif
	photo.Width = meta.Width
	photo.Height = meta.Height
	photo.DurationMS = meta.DurationMS
	return true, false, nil
}

type capturedMetadataContextUpdater interface {
	UpdatePhotoCapturedMetadataContext(context.Context, int64, int64, time.Time, *storage.PhotoEXIF, int, int, int64) error
}

func updateCapturedMetadata(ctx context.Context, repo storage.Repository, id, userID int64, takenAt time.Time, exif *storage.PhotoEXIF, width, height int, durationMS int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if updater, ok := repo.(capturedMetadataContextUpdater); ok {
		return updater.UpdatePhotoCapturedMetadataContext(ctx, id, userID, takenAt, exif, width, height, durationMS)
	}
	return repo.UpdatePhotoCapturedMetadata(id, userID, takenAt, exif, width, height, durationMS)
}

func imageMetadataUnchanged(photo *storage.Photo, takenAt time.Time, exif *storage.PhotoEXIF, width int, height int) bool {
	if photo == nil {
		return false
	}
	return timestampsEqual(photo.TakenAt, takenAt) &&
		photo.Width == width &&
		photo.Height == height &&
		reflect.DeepEqual(photo.EXIF, exif)
}

func videoMetadataUnchanged(photo *storage.Photo, takenAt time.Time, exif *storage.PhotoEXIF, width int, height int, durationMS int64) bool {
	if photo == nil {
		return false
	}
	return timestampsEqual(photo.TakenAt, takenAt) &&
		photo.Width == width &&
		photo.Height == height &&
		photo.DurationMS == durationMS &&
		reflect.DeepEqual(photo.EXIF, exif)
}

func timestampsEqual(a, b time.Time) bool {
	if a.IsZero() && b.IsZero() {
		return true
	}
	if a.IsZero() || b.IsZero() {
		return false
	}
	diff := a.Sub(b)
	if diff < 0 {
		diff = -diff
	}
	return diff <= time.Second
}

func (t *exifBackfillTask) update(fn func(status *EXIFBackfillStatus)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	fn(&t.status)
}

func (t *exifBackfillTask) finish(status, message, errText string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	t.status.Status = status
	t.status.Message = message
	t.status.Error = errText
	t.status.UpdatedAt = now
	t.status.FinishedAt = now
}

func (t *exifBackfillTask) snapshot() EXIFBackfillStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	snapshot := t.status
	snapshot.Errors = append([]string(nil), t.status.Errors...)
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
