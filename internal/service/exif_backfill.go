package service

import (
	"fmt"
	"os"
	"reflect"
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

// BackfillPhotoEXIF 修正旧媒体的 EXIF、拍摄时间与基础元数据。
func (s *PhotoService) BackfillPhotoEXIF(userID int64) (*EXIFBackfillResult, error) {
	result := &EXIFBackfillResult{}
	cursor := ""
	for {
		page, err := s.repo.ListPhotos(storage.ListPhotosParams{
			UserID:    userID,
			Cursor:    cursor,
			Limit:     200,
			SkipTotal: true,
		})
		if err != nil {
			return result, err
		}
		for _, photo := range page.Photos {
			result.Scanned++
			updated, skipped, err := s.repairMediaMetadata(photo, userID)
			if err != nil {
				result.Failed++
				if len(result.Errors) < 8 {
					name := ""
					if photo != nil {
						name = photo.OriginalName
					}
					if name == "" {
						name = "unknown media"
					}
					result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", name, err))
				}
				continue
			}
			if skipped {
				result.Skipped++
				continue
			}
			if updated {
				result.Updated++
			}
		}
		if !page.HasMore || page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	return result, nil
}

func (s *PhotoService) repairMediaMetadata(photo *storage.Photo, userID int64) (updated bool, skipped bool, err error) {
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
		return s.repairVideoMetadata(photo, userID, path, info.ModTime())
	default:
		return s.repairImageMetadata(photo, userID, path, info.ModTime())
	}
}

func (s *PhotoService) repairImageMetadata(photo *storage.Photo, userID int64, path string, fallbackTime time.Time) (bool, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, false, err
	}
	meta, err := imgpkg.ExtractMeta(file, photo.OriginalName, fallbackTime)
	_ = file.Close()
	if err != nil {
		return false, false, err
	}
	if meta == nil {
		return false, true, nil
	}
	nextExif := copyPhotoEXIF(meta.EXIF)
	nextTakenAt := meta.TakenAt
	if imageMetadataUnchanged(photo, nextTakenAt, nextExif, meta.Width, meta.Height) {
		return false, true, nil
	}
	if err := s.repo.UpdatePhotoCapturedMetadata(photo.ID, userID, nextTakenAt, nextExif, meta.Width, meta.Height, photo.DurationMS); err != nil {
		return false, false, err
	}
	photo.TakenAt = nextTakenAt
	photo.EXIF = nextExif
	photo.Width = meta.Width
	photo.Height = meta.Height
	return true, false, nil
}

func (s *PhotoService) repairVideoMetadata(photo *storage.Photo, userID int64, path string, fallbackTime time.Time) (bool, bool, error) {
	meta, err := media.ProbeVideo(path)
	if err != nil {
		return false, false, err
	}
	if meta == nil {
		return false, true, nil
	}
	nextTakenAt := fallbackTime
	if !meta.TakenAt.IsZero() {
		nextTakenAt = meta.TakenAt
	}
	nextExif := videoMetaEXIF(meta)
	if videoMetadataUnchanged(photo, nextTakenAt, nextExif, meta.Width, meta.Height, meta.DurationMS) {
		return false, true, nil
	}
	if err := s.repo.UpdatePhotoCapturedMetadata(photo.ID, userID, nextTakenAt, nextExif, meta.Width, meta.Height, meta.DurationMS); err != nil {
		return false, false, err
	}
	photo.TakenAt = nextTakenAt
	photo.EXIF = nextExif
	photo.Width = meta.Width
	photo.Height = meta.Height
	photo.DurationMS = meta.DurationMS
	return true, false, nil
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
