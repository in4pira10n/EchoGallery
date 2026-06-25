package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"echogallery/internal/media"
	"echogallery/internal/storage"
)

type RegisterUploadedVideoInput struct {
	UUID           string
	OriginalName   string
	MimeType       string
	Size           int64
	UploadedBy     int64
	TakenAt        time.Time
	StorageRelPath string
	SourceRelPath  string
	SourceModUnix  int64
	Meta           *media.VideoMeta
}

type VideoThumbnailRefreshResult struct {
	Total     int      `json:"total"`
	Refreshed int      `json:"refreshed"`
	Failed    int      `json:"failed"`
	Errors    []string `json:"errors,omitempty"`
}

type PlaybackCacheEntry struct {
	UUID         string    `json:"uuid"`
	OriginalName string    `json:"original_name"`
	SourcePath   string    `json:"source_path,omitempty"`
	CachePath    string    `json:"cache_path"`
	MimeType     string    `json:"mime_type"`
	Size         int64     `json:"size"`
	CreatedAt    time.Time `json:"created_at"`
}

type PlaybackCacheBuildStatus struct {
	Status    string `json:"status"`
	Message   string `json:"message"`
	Done      int    `json:"done"`
	Total     int    `json:"total"`
	Generated int    `json:"generated"`
	Skipped   int    `json:"skipped"`
	Failed    int    `json:"failed"`
	Error     string `json:"error,omitempty"`
}

type playbackCacheBuildTask struct {
	cancel     atomic.Bool
	cancelFunc context.CancelFunc
	ctx        context.Context
	status     atomic.Value
}

func newPlaybackCacheBuildTask() *playbackCacheBuildTask {
	ctx, cancel := context.WithCancel(context.Background())
	task := &playbackCacheBuildTask{ctx: ctx, cancelFunc: cancel}
	task.status.Store(PlaybackCacheBuildStatus{Status: "running", Message: "正在准备播放兼容缓存…"})
	return task
}

func (t *playbackCacheBuildTask) snapshot() PlaybackCacheBuildStatus {
	if t == nil {
		return PlaybackCacheBuildStatus{Status: "idle", Message: "当前没有播放兼容缓存任务"}
	}
	if status, ok := t.status.Load().(PlaybackCacheBuildStatus); ok {
		return status
	}
	return PlaybackCacheBuildStatus{Status: "idle", Message: "当前没有播放兼容缓存任务"}
}

func (t *playbackCacheBuildTask) update(fn func(status *PlaybackCacheBuildStatus)) {
	if t == nil || fn == nil {
		return
	}
	status := t.snapshot()
	fn(&status)
	t.status.Store(status)
}

func (s *PhotoService) RegisterUploadedVideo(input RegisterUploadedVideoInput) (*storage.Photo, error) {
	if input.UUID == "" {
		return nil, fmt.Errorf("UUID 不能为空")
	}
	if input.Meta == nil {
		return nil, fmt.Errorf("视频元数据不能为空")
	}
	if input.MimeType == "" {
		input.MimeType = "video/mp4"
	}
	if input.TakenAt.IsZero() && input.Meta != nil && !input.Meta.TakenAt.IsZero() {
		input.TakenAt = input.Meta.TakenAt
	}
	if input.TakenAt.IsZero() {
		input.TakenAt = time.Now()
	}

	photo := &storage.Photo{
		UUID:           input.UUID,
		OriginalName:   input.OriginalName,
		MediaKind:      storage.MediaKindVideo,
		MimeType:       input.MimeType,
		Size:           input.Size,
		Width:          input.Meta.Width,
		Height:         input.Meta.Height,
		DurationMS:     input.Meta.DurationMS,
		EXIF:           videoMetaEXIF(input.Meta),
		StorageRelPath: input.StorageRelPath,
		SourceRelPath:  input.SourceRelPath,
		SourceModUnix:  input.SourceModUnix,
		TakenAt:        input.TakenAt,
		UploadedAt:     time.Now(),
		UploadedBy:     input.UploadedBy,
	}

	if err := s.repo.SavePhoto(photo); err != nil {
		return nil, fmt.Errorf("保存视频记录失败: %w", err)
	}
	if s.syncThumbnail {
		_ = s.generateThumbnailForPhoto(photo)
	} else {
		go s.enqueueThumbnailGeneration(photo, true)
	}
	return photo, nil
}

func videoMetaEXIF(meta *media.VideoMeta) *storage.PhotoEXIF {
	if meta == nil {
		return nil
	}
	exif := &storage.PhotoEXIF{
		Make:           strings.TrimSpace(meta.Make),
		Model:          strings.TrimSpace(meta.Model),
		VideoCodec:     strings.TrimSpace(meta.CodecName),
		VideoFrameRate: meta.FrameRate,
		TakenAt:        meta.TakenAt,
	}
	if exif.Make == "" && exif.Model == "" && exif.VideoCodec == "" && exif.VideoFrameRate <= 0 && exif.TakenAt.IsZero() {
		return nil
	}
	return exif
}

func (s *PhotoService) MediaPath(photo *storage.Photo) string {
	if photo.SourceRelPath != "" {
		sourcePath := filepath.Join(s.sourcePath, photo.SourceRelPath)
		if _, err := os.Stat(sourcePath); err == nil {
			return sourcePath
		}
	}
	if photo.StorageRelPath != "" {
		return filepath.Join(s.dataPath, photo.StorageRelPath)
	}
	ext := filepath.Ext(photo.OriginalName)
	return filepath.Join(s.dataPath, photo.UUID+ext)
}

func (s *PhotoService) BrowserPlaybackPath(photo *storage.Photo) (string, string, error) {
	return s.browserPlaybackPathWithContext(context.Background(), photo, false)
}

func (s *PhotoService) browserPlaybackPathWithContext(ctx context.Context, photo *storage.Photo, allowBuild bool) (string, string, error) {
	if photo == nil {
		return "", "", fmt.Errorf("media is empty")
	}
	sourcePath := s.MediaPath(photo)
	mode := browserPlaybackMode(photo)
	if mode == "direct" {
		return sourcePath, strings.TrimSpace(photo.MimeType), nil
	}
	cachePath := s.browserPlaybackCachePath(photo, sourcePath)
	if info, err := os.Stat(cachePath); err == nil && info.Size() > 0 {
		return cachePath, "video/mp4", nil
	}
	if !allowBuild {
		return "", "", fmt.Errorf("browser playback cache is missing")
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return "", "", fmt.Errorf("failed to create playback cache directory: %w", err)
	}
	tmpPath := cachePath + ".tmp"
	_ = os.Remove(tmpPath)
	err := media.RemuxToMP4ForBrowserContext(ctx, sourcePath, tmpPath)
	if err != nil && ctx != nil && ctx.Err() != nil {
		_ = os.Remove(tmpPath)
		return "", "", ctx.Err()
	}
	if err != nil && mode == "transcode" {
		err = media.TranscodeToMP4ForBrowserContext(ctx, sourcePath, tmpPath)
	} else if err != nil && mode == "remux" {
		err = media.TranscodeToMP4ForBrowserContext(ctx, sourcePath, tmpPath)
	}
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", "", err
	}
	if err := os.Rename(tmpPath, cachePath); err != nil {
		_ = os.Remove(tmpPath)
		return "", "", fmt.Errorf("failed to store playback cache: %w", err)
	}
	return cachePath, "video/mp4", nil
}

func (s *PhotoService) BuildBrowserPlaybackCache(photoID int64, userID int64) (*PlaybackCacheEntry, error) {
	photo, err := s.GetPhoto(photoID, userID)
	if err != nil {
		return nil, err
	}
	if photo == nil {
		return nil, fmt.Errorf("照片/视频不存在")
	}
	if photo.MediaKind != storage.MediaKindVideo {
		return nil, fmt.Errorf("当前媒体不是视频")
	}
	sourcePath := s.MediaPath(photo)
	playbackPath, mimeType, err := s.browserPlaybackPathWithContext(context.Background(), photo, true)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(playbackPath)
	if err != nil {
		return nil, err
	}
	return &PlaybackCacheEntry{
		UUID:         photo.UUID,
		OriginalName: photo.OriginalName,
		SourcePath:   sourcePath,
		CachePath:    playbackPath,
		MimeType:     mimeType,
		Size:         info.Size(),
		CreatedAt:    info.ModTime(),
	}, nil
}

func (s *PhotoService) browserPlaybackCachePath(photo *storage.Photo, sourcePath string) string {
	modUnix := photo.SourceModUnix
	if modUnix <= 0 {
		if info, err := os.Stat(sourcePath); err == nil {
			modUnix = info.ModTime().Unix()
		}
	}
	fileName := fmt.Sprintf("%s-%d-%d.mp4", photo.UUID, photo.Size, modUnix)
	return filepath.Join(s.dataPath, "playback", fileName)
}

func browserPlaybackMode(photo *storage.Photo) string {
	ext := strings.ToLower(filepath.Ext(photo.OriginalName))
	mimeType := strings.ToLower(strings.TrimSpace(photo.MimeType))
	if ext == ".mkv" || strings.Contains(mimeType, "matroska") {
		return "remux"
	}
	switch ext {
	case ".avi", ".wmv", ".wma", ".mpg", ".mpeg", ".ts", ".mts", ".m2ts", ".ogv":
		return "transcode"
	}
	if strings.Contains(mimeType, "x-ms-wmv") || strings.Contains(mimeType, "x-ms-wma") || strings.Contains(mimeType, "x-msvideo") {
		return "transcode"
	}
	return "direct"
}

func needsBrowserPlaybackCache(photo *storage.Photo) bool {
	return browserPlaybackMode(photo) != "direct"
}

func (s *PhotoService) ListPlaybackCaches(userID int64) ([]PlaybackCacheEntry, error) {
	entries := []PlaybackCacheEntry{}
	cursor := ""
	for {
		page, err := s.repo.ListPhotos(storage.ListPhotosParams{
			UserID:    userID,
			Cursor:    cursor,
			Limit:     300,
			SkipTotal: true,
			MediaKind: storage.MediaKindVideo,
		})
		if err != nil {
			return nil, err
		}
		if page == nil {
			break
		}
		for _, photo := range page.Photos {
			if photo == nil || !needsBrowserPlaybackCache(photo) {
				continue
			}
			sourcePath := s.MediaPath(photo)
			cachePath := s.browserPlaybackCachePath(photo, sourcePath)
			info, err := os.Stat(cachePath)
			if err != nil || info.IsDir() {
				continue
			}
			entries = append(entries, PlaybackCacheEntry{
				UUID:         photo.UUID,
				OriginalName: photo.OriginalName,
				SourcePath:   sourcePath,
				CachePath:    cachePath,
				MimeType:     photo.MimeType,
				Size:         info.Size(),
				CreatedAt:    info.ModTime(),
			})
		}
		if !page.HasMore || page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	return entries, nil
}

func (s *PhotoService) DeletePlaybackCaches(userID int64, uuids []string) (int, error) {
	allowed := map[string]struct{}{}
	for _, uuid := range uuids {
		uuid = strings.TrimSpace(uuid)
		if uuid != "" {
			allowed[uuid] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return 0, nil
	}
	entries, err := s.ListPlaybackCaches(userID)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, entry := range entries {
		if _, ok := allowed[entry.UUID]; !ok {
			continue
		}
		if err := os.Remove(entry.CachePath); err == nil || os.IsNotExist(err) {
			deleted++
		} else {
			return deleted, err
		}
	}
	return deleted, nil
}

func (s *PhotoService) StartPlaybackCacheBuild(userID int64) PlaybackCacheBuildStatus {
	s.playbackBuildMu.Lock()
	if s.playbackBuild != nil {
		status := s.playbackBuild.snapshot()
		if status.Status == "running" || status.Status == "cancelling" {
			s.playbackBuildMu.Unlock()
			return status
		}
	}
	task := newPlaybackCacheBuildTask()
	s.playbackBuild = task
	s.playbackBuildMu.Unlock()
	go s.runPlaybackCacheBuild(task, userID)
	return task.snapshot()
}

func (s *PhotoService) GetPlaybackCacheBuildStatus(userID int64) PlaybackCacheBuildStatus {
	s.playbackBuildMu.Lock()
	task := s.playbackBuild
	s.playbackBuildMu.Unlock()
	if task == nil {
		return PlaybackCacheBuildStatus{Status: "idle", Message: "当前没有播放兼容缓存任务"}
	}
	return task.snapshot()
}

func (s *PhotoService) CancelPlaybackCacheBuild(userID int64) PlaybackCacheBuildStatus {
	s.playbackBuildMu.Lock()
	task := s.playbackBuild
	s.playbackBuildMu.Unlock()
	if task == nil {
		return PlaybackCacheBuildStatus{Status: "idle", Message: "当前没有播放兼容缓存任务"}
	}
	task.cancel.Store(true)
	if task.cancelFunc != nil {
		task.cancelFunc()
	}
	task.update(func(status *PlaybackCacheBuildStatus) {
		if status.Status == "running" {
			status.Status = "cancelling"
			status.Message = "正在取消播放兼容缓存任务…"
		}
	})
	return task.snapshot()
}

func (s *PhotoService) BuildPlaybackCachesSync(userID int64, progress func(PlaybackCacheBuildStatus) bool) PlaybackCacheBuildStatus {
	return s.BuildPlaybackCachesSyncContext(context.Background(), userID, progress)
}

func (s *PhotoService) BuildPlaybackCachesSyncContext(ctx context.Context, userID int64, progress func(PlaybackCacheBuildStatus) bool) PlaybackCacheBuildStatus {
	task := newPlaybackCacheBuildTask()
	done := make(chan struct{})
	if ctx != nil {
		go func() {
			select {
			case <-ctx.Done():
				task.cancel.Store(true)
				if task.cancelFunc != nil {
					task.cancelFunc()
				}
			case <-done:
			}
		}()
	}
	s.runPlaybackCacheBuildWithProgress(task, userID, progress)
	close(done)
	return task.snapshot()
}

func (s *PhotoService) runPlaybackCacheBuild(task *playbackCacheBuildTask, userID int64) {
	s.runPlaybackCacheBuildWithProgress(task, userID, nil)
}

func (s *PhotoService) runPlaybackCacheBuildWithProgress(task *playbackCacheBuildTask, userID int64, progress func(PlaybackCacheBuildStatus) bool) {
	cursor := ""
	total := 0
	for {
		page, err := s.repo.ListPhotos(storage.ListPhotosParams{
			UserID:    userID,
			Cursor:    cursor,
			Limit:     300,
			SkipTotal: cursor != "",
			MediaKind: storage.MediaKindVideo,
		})
		if err != nil {
			task.update(func(status *PlaybackCacheBuildStatus) {
				status.Status = "failed"
				status.Message = "播放兼容缓存构建失败"
				status.Error = err.Error()
			})
			return
		}
		if page == nil {
			break
		}
		if cursor == "" && page.Total > 0 {
			total = page.Total
			task.update(func(status *PlaybackCacheBuildStatus) {
				status.Total = total
			})
		}
		for _, photo := range page.Photos {
			if task.cancel.Load() {
				task.update(func(status *PlaybackCacheBuildStatus) {
					status.Status = "cancelled"
					status.Message = "已取消播放兼容缓存任务"
				})
				return
			}
			if photo == nil || !needsBrowserPlaybackCache(photo) {
				task.update(func(status *PlaybackCacheBuildStatus) {
					status.Done++
					status.Skipped++
					status.Message = "跳过浏览器已支持的视频"
				})
				continue
			}
			task.update(func(status *PlaybackCacheBuildStatus) {
				status.Message = fmt.Sprintf("正在准备 %s 的播放缓存", photo.OriginalName)
			})
			_, _, err := s.browserPlaybackPathWithContext(task.ctx, photo, true)
			if err != nil && task.cancel.Load() {
				task.update(func(status *PlaybackCacheBuildStatus) {
					status.Status = "cancelled"
					status.Message = "已取消播放兼容缓存任务"
				})
				return
			}
			task.update(func(status *PlaybackCacheBuildStatus) {
				status.Done++
				if err != nil {
					status.Failed++
					status.Message = fmt.Sprintf("%s 播放缓存构建失败", photo.OriginalName)
					status.Error = err.Error()
				} else {
					status.Generated++
					status.Message = fmt.Sprintf("%s 播放缓存已就绪", photo.OriginalName)
				}
			})
			if progress != nil && !progress(task.snapshot()) {
				task.cancel.Store(true)
			}
		}
		if !page.HasMore || page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	task.update(func(status *PlaybackCacheBuildStatus) {
		if status.Status == "running" || status.Status == "" {
			status.Status = "completed"
			status.Message = "播放兼容缓存构建完成"
		}
	})
}

func (s *PhotoService) PosterPath(photo *storage.Photo) string {
	return media.PosterPath(s.managedThumbnailRoot(), photo.UUID)
}

func (s *PhotoService) RefreshVideoThumbnails(userID int64) (*VideoThumbnailRefreshResult, error) {
	result := &VideoThumbnailRefreshResult{}
	for _, trashed := range []bool{false, true} {
		cursor := ""
		for {
			params := storage.ListPhotosParams{UserID: userID, Cursor: cursor, Limit: 200}
			var page *storage.PhotoPage
			var err error
			if trashed {
				page, err = s.repo.ListTrashedPhotos(params)
			} else {
				page, err = s.repo.ListPhotos(params)
			}
			if err != nil {
				return nil, err
			}
			for _, photo := range page.Photos {
				if photo.MediaKind != storage.MediaKindVideo {
					continue
				}
				result.Total++
				if err := s.generateThumbnailForPhoto(photo); err != nil {
					result.Failed++
					if len(result.Errors) < 8 {
						result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", photo.OriginalName, err))
					}
					continue
				}
				result.Refreshed++
			}
			if !page.HasMore || page.NextCursor == "" {
				break
			}
			cursor = page.NextCursor
		}
	}
	return result, nil
}
