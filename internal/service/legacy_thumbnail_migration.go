package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	imgpkg "echogallery/internal/image"
	"echogallery/internal/storage"
	"golang.org/x/image/webp"
)

type LegacyThumbnailResult struct {
	Copied   int
	Existing int
	Resumed  int
	Missing  int
	Cleaned  int
}

type LegacyThumbnailProgress struct {
	Done     int
	Total    int
	Result   LegacyThumbnailResult
	Verified map[string]string
}

type ThumbnailRepairResult struct {
	Generated int
	Failed    int
	Errors    []string
}

func validWebPThumbnail(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := webp.DecodeConfig(file)
	return err == nil && info.Width > 0 && info.Height > 0
}

func thumbnailChecksum(path string) ([32]byte, error) {
	var empty [32]byte
	file, err := os.Open(path)
	if err != nil {
		return empty, err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return empty, err
	}
	var sum [32]byte
	copy(sum[:], hash.Sum(nil))
	return sum, nil
}

func (s *PhotoService) oldThumbnailPaths(id string) []string {
	if s == nil || s.legacyThumbnailRoot == "" {
		return nil
	}
	roots := []string{s.legacyManagedThumbnailRoot(), s.legacyThumbnailRoot}
	paths := make([]string, 0, len(roots)*2)
	for _, root := range roots {
		paths = append(paths, imgpkg.ThumbnailShardPath(root, id), imgpkg.ThumbnailFlatPath(root, id))
	}
	return paths
}

func (s *PhotoService) oldThumbnailCleanupPaths(id string) []string {
	paths := s.oldThumbnailPaths(id)
	for _, root := range []string{s.legacyManagedThumbnailRoot(), s.legacyThumbnailRoot} {
		paths = append(paths,
			filepath.Join(root, id+".jpg"),
			filepath.Join(root, id+".preview.webp"),
			filepath.Join(root, id+".build-preview.webp"),
		)
	}
	return paths
}

func legacyThumbnailWorkerCount() int {
	if lowResourceModeEnabled() {
		return 2
	}
	count := runtime.GOMAXPROCS(0)
	if count > 8 {
		count = 8
	}
	if count < 2 {
		count = 2
	}
	return count
}

func (s *PhotoService) migrateLegacyThumbnailFile(match storage.LegacyUUIDMatch, completed map[string]string) (string, error) {
	target := imgpkg.ThumbnailShardPath(s.managedThumbnailRoot(), match.OldUUID)
	if completed[match.OldUUID] == match.SourceRelPath && validWebPThumbnail(target) {
		return "resumed", nil
	}
	if validWebPThumbnail(target) {
		return "existing", nil
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("移除损坏的新缩略图失败 %s: %w", match.SourceRelPath, err)
	}
	for _, source := range s.oldThumbnailPaths(match.OldUUID) {
		if !validWebPThumbnail(source) {
			continue
		}
		oldSum, err := copyManagedThumbnailFileWithChecksum(source, target)
		if err != nil {
			return "", fmt.Errorf("复制 %s 的旧缩略图失败: %w", match.SourceRelPath, err)
		}
		if !validWebPThumbnail(target) {
			return "", fmt.Errorf("复制后缩略图无法解码: %s", match.SourceRelPath)
		}
		newSum, err := thumbnailChecksum(target)
		if err != nil || oldSum != newSum {
			return "", fmt.Errorf("复制后缩略图校验失败: %s", match.SourceRelPath)
		}
		return "copied", nil
	}
	return "missing", nil
}

func (s *PhotoService) MigrateLegacyThumbnailFiles(ctx context.Context, matches []storage.LegacyUUIDMatch, completed map[string]string, progress func(LegacyThumbnailProgress) error) (LegacyThumbnailResult, error) {
	var result LegacyThumbnailResult
	if s == nil || s.legacyThumbnailRoot == "" {
		return result, fmt.Errorf("旧版缩略图目录不可用")
	}
	type outcome struct {
		match storage.LegacyUUIDMatch
		kind  string
		err   error
	}
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan storage.LegacyUUIDMatch)
	results := make(chan outcome, legacyThumbnailWorkerCount())
	var workers sync.WaitGroup
	for range legacyThumbnailWorkerCount() {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for match := range jobs {
				if workerCtx.Err() != nil {
					return
				}
				kind, err := s.migrateLegacyThumbnailFile(match, completed)
				results <- outcome{match: match, kind: kind, err: err}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, match := range matches {
			select {
			case <-workerCtx.Done():
				return
			case jobs <- match:
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()
	verified := make(map[string]string, 64)
	done := 0
	var firstErr error
	for item := range results {
		done++
		if item.err != nil && firstErr == nil {
			firstErr = item.err
			cancel()
		}
		switch item.kind {
		case "copied":
			result.Copied++
		case "existing":
			result.Existing++
		case "resumed":
			result.Resumed++
		case "missing":
			result.Missing++
		}
		if item.err == nil && item.kind != "missing" {
			verified[item.match.OldUUID] = item.match.SourceRelPath
		}
		if progress != nil && (len(verified) >= 64 || done%64 == 0 || done == len(matches)) {
			if err := progress(LegacyThumbnailProgress{Done: done, Total: len(matches), Result: result, Verified: verified}); err != nil && firstErr == nil {
				firstErr = err
				cancel()
			}
			verified = make(map[string]string, 64)
		}
	}
	if progress != nil && len(verified) > 0 && firstErr == nil {
		firstErr = progress(LegacyThumbnailProgress{Done: done, Total: len(matches), Result: result, Verified: verified})
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	return result, firstErr
}

func (s *PhotoService) AuditNewThumbnails(ctx context.Context, userID int64) (int, error) {
	return s.AuditNewThumbnailsWithProgress(ctx, userID, nil)
}

func (s *PhotoService) AuditNewThumbnailsWithProgress(ctx context.Context, userID int64, progress func(int, int)) (int, error) {
	photos, err := s.MissingNewThumbnailsWithProgress(ctx, userID, progress)
	return len(photos), err
}

func (s *PhotoService) MissingNewThumbnailsWithProgress(ctx context.Context, userID int64, progress func(int, int)) ([]*storage.Photo, error) {
	var missing []*storage.Photo
	done := 0
	total := 0
	cursor := ""
	for {
		if err := ctx.Err(); err != nil {
			return missing, err
		}
		page, err := s.repo.ListPhotos(storage.ListPhotosParams{UserID: userID, Cursor: cursor, Limit: 320})
		if err != nil {
			return missing, err
		}
		if page == nil {
			break
		}
		if total == 0 {
			total = page.Total
		}
		for _, photo := range page.Photos {
			if !validWebPThumbnail(s.ThumbnailPath(photo)) {
				missing = append(missing, photo)
			}
			done++
		}
		if progress != nil {
			progress(done, total)
		}
		if !page.HasMore || page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	return missing, nil
}

func (s *PhotoService) RebuildMissingThumbnails(ctx context.Context, photos []*storage.Photo, progress func(int, int)) (ThumbnailRepairResult, error) {
	var result ThumbnailRepairResult
	for index, photo := range photos {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		target := s.ThumbnailPath(photo)
		if validWebPThumbnail(target) {
			continue
		}
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", photo.SourceRelPath, err))
			continue
		}
		if err := s.writeThumbnailForPhoto(photo); err != nil || !validWebPThumbnail(target) {
			if err == nil {
				err = fmt.Errorf("生成的缩略图无法解码")
			}
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", photo.SourceRelPath, err))
		} else {
			result.Generated++
		}
		if progress != nil {
			progress(index+1, len(photos))
		}
	}
	return result, nil
}

func (s *PhotoService) RemoveCorruptNewThumbnails(ctx context.Context, userID int64, alreadyVerified map[string]struct{}) (int, error) {
	removed := 0
	cursor := ""
	for {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		page, err := s.repo.ListPhotos(storage.ListPhotosParams{UserID: userID, Cursor: cursor, Limit: 320})
		if err != nil {
			return removed, err
		}
		if page == nil {
			break
		}
		for _, photo := range page.Photos {
			if _, verified := alreadyVerified[photo.UUID]; verified {
				continue
			}
			target := s.ThumbnailPath(photo)
			if _, err := os.Stat(target); os.IsNotExist(err) {
				continue
			} else if err != nil {
				return removed, err
			}
			if validWebPThumbnail(target) {
				continue
			}
			if err := os.Remove(target); err != nil {
				return removed, fmt.Errorf("移除损坏的新版缩略图失败 %s: %w", photo.SourceRelPath, err)
			}
			removed++
		}
		if !page.HasMore || page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	return removed, nil
}

func (s *PhotoService) CleanCopiedLegacyThumbnails(ctx context.Context, matches []storage.LegacyUUIDMatch) (int, error) {
	return s.CleanCopiedLegacyThumbnailsWithProgress(ctx, matches, nil)
}

func (s *PhotoService) CleanCopiedLegacyThumbnailsWithProgress(ctx context.Context, matches []storage.LegacyUUIDMatch, progress func(int, int)) (int, error) {
	cleaned := 0
	for index, match := range matches {
		if err := ctx.Err(); err != nil {
			return cleaned, err
		}
		target := imgpkg.ThumbnailShardPath(s.managedThumbnailRoot(), match.OldUUID)
		if !validWebPThumbnail(target) {
			return cleaned, fmt.Errorf("新缩略图未验证，保留旧文件: %s", match.SourceRelPath)
		}
		for _, oldPath := range s.oldThumbnailCleanupPaths(match.OldUUID) {
			if oldPath == target {
				continue
			}
			if err := os.Remove(oldPath); err == nil {
				cleaned++
			} else if !os.IsNotExist(err) {
				return cleaned, fmt.Errorf("清理旧缩略图失败 %s: %w", filepath.Base(oldPath), err)
			}
		}
		if progress != nil && ((index+1)%64 == 0 || index+1 == len(matches)) {
			progress(index+1, len(matches))
		}
	}
	return cleaned, nil
}
