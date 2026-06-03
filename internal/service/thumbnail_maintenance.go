package service

import (
	"context"
	"fmt"
	"os"

	"echogallery/internal/storage"
)

type ThumbnailMaintenanceOptions struct {
	MoveLegacyThumbnails bool
	CleanThumbnailFiles  bool
}

type ThumbnailMaintenanceProgress struct {
	Done    int
	Total   int
	Moved   int
	Cleaned int
	Skipped int
	Failed  int
	Message string
}

type ThumbnailMaintenanceSummary struct {
	Done    int
	Total   int
	Moved   int
	Cleaned int
	Skipped int
	Failed  int
}

func (o ThumbnailMaintenanceOptions) Enabled() bool {
	return o.MoveLegacyThumbnails || o.CleanThumbnailFiles
}

func (s *PhotoService) MaintainThumbnailsContext(ctx context.Context, userID int64, options ThumbnailMaintenanceOptions, progress func(ThumbnailMaintenanceProgress)) (ThumbnailMaintenanceSummary, error) {
	var summary ThumbnailMaintenanceSummary
	if s == nil || !options.Enabled() {
		return summary, nil
	}
	report := func(message string) {
		if progress == nil {
			return
		}
		progress(ThumbnailMaintenanceProgress{
			Done:    summary.Done,
			Total:   summary.Total,
			Moved:   summary.Moved,
			Cleaned: summary.Cleaned,
			Skipped: summary.Skipped,
			Failed:  summary.Failed,
			Message: message,
		})
	}
	for _, trashed := range []bool{false, true} {
		cursor := ""
		firstPage := true
		for {
			if err := ctx.Err(); err != nil {
				return summary, err
			}
			params := storage.ListPhotosParams{
				UserID:    userID,
				Cursor:    cursor,
				Limit:     320,
				SkipTotal: !firstPage,
			}
			var (
				page *storage.PhotoPage
				err  error
			)
			if trashed {
				page, err = s.repo.ListTrashedPhotos(params)
			} else {
				page, err = s.repo.ListPhotos(params)
			}
			if err != nil {
				return summary, err
			}
			if page == nil {
				break
			}
			if firstPage {
				summary.Total += page.Total
				firstPage = false
				report(thumbnailMaintenanceMessage(options, summary))
			}
			for _, photo := range page.Photos {
				if err := ctx.Err(); err != nil {
					return summary, err
				}
				moved, cleaned, changed, err := s.maintainThumbnailForPhoto(photo, options)
				summary.Done++
				if err != nil {
					summary.Failed++
				} else {
					summary.Moved += moved
					summary.Cleaned += cleaned
					if !changed {
						summary.Skipped++
					}
				}
				report(thumbnailMaintenanceMessage(options, summary))
			}
			if !page.HasMore || page.NextCursor == "" {
				break
			}
			cursor = page.NextCursor
		}
	}
	return summary, nil
}

func thumbnailMaintenanceMessage(options ThumbnailMaintenanceOptions, summary ThumbnailMaintenanceSummary) string {
	switch {
	case options.MoveLegacyThumbnails && options.CleanThumbnailFiles:
		return fmt.Sprintf("正在整理旧版资源库… 已处理 %d 项，迁移 %d，清理 %d", summary.Done, summary.Moved, summary.Cleaned)
	case options.MoveLegacyThumbnails:
		return fmt.Sprintf("正在整理旧版资源库… 已处理 %d 项，迁移 %d", summary.Done, summary.Moved)
	case options.CleanThumbnailFiles:
		return fmt.Sprintf("正在清理缩略图文件… 已处理 %d 项，清理 %d", summary.Done, summary.Cleaned)
	default:
		return fmt.Sprintf("正在整理缩略图目录… 已处理 %d 项", summary.Done)
	}
}

func (s *PhotoService) maintainThumbnailForPhoto(photo *storage.Photo, options ThumbnailMaintenanceOptions) (moved int, cleaned int, changed bool, err error) {
	if photo == nil {
		return 0, 0, false, nil
	}
	targetPath := s.ThumbnailPath(photo)
	targetExists := resolveManagedFile(targetPath)
	legacyShardedPath := s.legacyShardedThumbnailPath(photo)
	legacyFlatPath := s.legacyFlatThumbnailPath(photo)
	if options.MoveLegacyThumbnails && !targetExists {
		for _, legacyPath := range []string{legacyShardedPath, legacyFlatPath} {
			if legacyPath == "" || legacyPath == targetPath || !resolveManagedFile(legacyPath) {
				continue
			}
			if err := relocateManagedThumbnailFile(legacyPath, targetPath); err != nil {
				return 0, 0, false, err
			}
			moved = 1
			targetExists = true
			break
		}
	}
	if options.CleanThumbnailFiles {
		cleaned = s.cleanupThumbnailArtifacts(photo, targetExists, true)
	}
	changed = moved > 0 || cleaned > 0
	return moved, cleaned, changed, nil
}

func removeThumbnailFileIfExists(path string) bool {
	if path == "" {
		return false
	}
	if err := os.Remove(path); err == nil {
		return true
	}
	return false
}
