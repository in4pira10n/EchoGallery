package service

import (
	"fmt"
	"os"

	imgpkg "echogallery/internal/image"
	"echogallery/internal/storage"
)

// EXIFBackfillResult 批量回填 EXIF 的结果。
type EXIFBackfillResult struct {
	Scanned int      `json:"scanned"`
	Updated int      `json:"updated"`
	Skipped int      `json:"skipped"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors,omitempty"`
}

// BackfillPhotoEXIF 回填旧照片的 EXIF 信息。
func (s *PhotoService) BackfillPhotoEXIF(userID int64) (*EXIFBackfillResult, error) {
	result := &EXIFBackfillResult{}
	cursor := ""
	for {
		page, err := s.repo.ListPhotos(storage.ListPhotosParams{
			UserID:    userID,
			Cursor:    cursor,
			Limit:     200,
			MediaKind: storage.MediaKindImage,
			SkipTotal: true,
		})
		if err != nil {
			return result, err
		}
		for _, photo := range page.Photos {
			result.Scanned++
			if photo == nil || photo.EXIF != nil {
				result.Skipped++
				continue
			}
			path := s.MediaPath(photo)
			info, err := os.Stat(path)
			if err != nil || info.IsDir() {
				result.Skipped++
				continue
			}
			file, err := os.Open(path)
			if err != nil {
				result.Failed++
				if len(result.Errors) < 8 {
					result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", photo.OriginalName, err))
				}
				continue
			}
			meta, err := imgpkg.ExtractMeta(file, photo.OriginalName, info.ModTime())
			_ = file.Close()
			if err != nil {
				result.Failed++
				if len(result.Errors) < 8 {
					result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", photo.OriginalName, err))
				}
				continue
			}
			if meta == nil || meta.EXIF == nil {
				result.Skipped++
				continue
			}
			if err := s.repo.UpdatePhotoEXIF(photo.ID, userID, copyPhotoEXIF(meta.EXIF)); err != nil {
				result.Failed++
				if len(result.Errors) < 8 {
					result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", photo.OriginalName, err))
				}
				continue
			}
			result.Updated++
		}
		if !page.HasMore || page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	return result, nil
}
