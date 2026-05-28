package service

import (
	"fmt"
	"os"
	"path/filepath"
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

func (s *PhotoService) PosterPath(photo *storage.Photo) string {
	return media.PosterPath(s.thumbnailPath, photo.UUID)
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
				if err := s.generateThumbnailTiersForPhoto(photo); err != nil {
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
