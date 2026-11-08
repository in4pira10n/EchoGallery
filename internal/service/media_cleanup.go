package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"

	"echogallery/internal/config"
	"echogallery/internal/danmaku"
	"echogallery/internal/storage"
)

type missingPhotoRepository interface {
	HardDeleteMissingPhoto(context.Context, *storage.Photo) error
}

type cleanupCacheDirectory struct {
	owned  map[string][]string
	legacy map[string]bool
}

// CleanMissingMediaContext is an explicit, optional audit after a successful
// import. It does not depend on snapshot freshness, nor change its format.
// All indexed paths are checked before anything is removed. Only ENOENT means
// missing; permission, IO, network and cancellation errors abort the audit.
func (s *PhotoService) CleanMissingMediaContext(ctx context.Context, userID int64, progress func(done, total int)) (int, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-s.workerStop:
			cancel()
		case <-ctx.Done():
		}
	}()
	for !s.scanMu.TryLock() {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(40 * time.Millisecond):
		}
	}
	defer s.scanMu.Unlock()
	repo, ok := s.repo.(missingPhotoRepository)
	if !ok {
		return 0, fmt.Errorf("repository does not support safe media cleanup")
	}
	userID = s.LibraryUserID(userID)
	rootInfo, err := scanStat(ctx, s.sourcePath)
	if err != nil {
		return 0, err
	}
	if !rootInfo.IsDir() {
		return 0, fmt.Errorf("library is not a directory: %s", s.sourcePath)
	}
	index, err := s.repo.ListSourceMediaIndex(userID)
	if err != nil {
		return 0, err
	}
	paths := make([]string, 0, len(index))
	for relative, entry := range index {
		// Trash has its own lifecycle; a moved-to-trash source is intentionally absent.
		if relative != "" && !entry.Deleted {
			paths = append(paths, relative)
		}
	}
	sort.Strings(paths)
	var missing []*storage.Photo
	checkedDirectories := make(map[string]map[string]bool)
	if progress != nil {
		progress(0, len(paths))
	}
	for i, relative := range paths {
		if !filepath.IsLocal(filepath.FromSlash(relative)) {
			return 0, fmt.Errorf("invalid indexed source path: %s", relative)
		}
		path := filepath.Join(s.sourcePath, filepath.FromSlash(relative))
		reportScan(ctx, "cleanup", "check_missing", path, -1, 0, 0)
		_, err := scanStat(ctx, path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return 0, err
		}
		if errors.Is(err, fs.ErrNotExist) {
			if err := s.confirmSourceAbsent(ctx, path, checkedDirectories); err != nil {
				return 0, err
			}
			photo, err := s.repo.GetPhotoByID(index[relative].ID, userID)
			if err != nil {
				return 0, err
			}
			if photo != nil {
				if photo.SourceRelPath != relative || photo.Size != index[relative].Size {
					return 0, fmt.Errorf("media changed during cleanup audit: %s", relative)
				}
				// Normal photo reads omit the private fingerprint column. The lightweight
				// index carries its exact nanosecond value, including legacy zero values.
				photo.SourceModUnix = index[relative].SourceModUnix
				if parsed, err := uuid.Parse(photo.UUID); err != nil || parsed.String() != photo.UUID {
					return 0, fmt.Errorf("invalid media cache identity: %s", photo.UUID)
				}
				missing = append(missing, photo)
			}
		}
		if progress != nil {
			progress(i+1, len(paths))
		}
	}
	if len(missing) == 0 {
		return 0, ctx.Err()
	}
	// Readability and identity are checked again after discovery. A disconnected
	// share or replaced mount must never turn into mass deletion of indexed media.
	if err := s.checkCleanupRoot(ctx, rootInfo); err != nil {
		return 0, err
	}
	if _, err := scanIO(ctx, s.sourcePath, func() ([]fs.DirEntry, error) { return os.ReadDir(s.sourcePath) }); err != nil {
		return 0, err
	}
	cacheRoots := []string{
		filepath.Join(s.dataPath, "playback"),
		filepath.Join(config.LibraryDataRoot(s.sourcePath), "soundtracks"),
		filepath.Join(config.LibraryDataRoot(s.sourcePath), "subtitles"),
	}
	caches := make(map[string]cleanupCacheDirectory)
	for _, root := range cacheRoots {
		owner := config.LibraryDataRoot(s.sourcePath)
		if root == filepath.Join(s.dataPath, "playback") {
			owner = s.dataPath
		}
		if err := checkCacheParent(ctx, owner, filepath.Join(root, ".cleanup-check")); err != nil {
			return 0, err
		}
		entries, err := scanIO(ctx, root, func() ([]fs.DirEntry, error) { return os.ReadDir(root) })
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return 0, err
		}
		directory := cleanupCacheDirectory{owned: make(map[string][]string), legacy: make(map[string]bool)}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if len(name) > 37 && name[36] == '-' {
				if parsed, err := uuid.Parse(name[:36]); err == nil && parsed.String() == name[:36] {
					directory.owned[name[:36]] = append(directory.owned[name[:36]], name)
					continue
				}
			}
			if len(name) == 68 && (strings.HasSuffix(name, ".mp4") || strings.HasSuffix(name, ".vtt")) && strings.Trim(name[:64], "0123456789abcdef") == "" {
				directory.legacy[name] = true
			}
		}
		caches[root] = directory
	}
	danmakuRoot := filepath.Join(config.LibraryDataRoot(s.sourcePath), "danmaku")
	if err := checkCacheParent(ctx, config.LibraryDataRoot(s.sourcePath), filepath.Join(danmakuRoot, ".cleanup-check")); err != nil {
		return 0, err
	}
	danmakuCleaner, err := scanIO(ctx, danmakuRoot, func() (*danmaku.MediaFileCleaner, error) {
		return danmaku.NewMediaFileCleaner(danmakuRoot)
	})
	if err != nil {
		return 0, err
	}
	cleaned := 0
	if progress != nil {
		progress(0, len(missing))
	}
	for _, photo := range missing {
		if err := s.checkCleanupRoot(ctx, rootInfo); err != nil {
			return cleaned, err
		}
		// A source can have reappeared while the audit was running.
		path := filepath.Join(s.sourcePath, filepath.FromSlash(photo.SourceRelPath))
		if _, err := scanStat(ctx, path); err == nil {
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return cleaned, err
		}
		// Discovery listings may be stale by now. Prove absence again immediately
		// before touching artifacts; a newly introduced dangling link or an
		// unavailable parent must preserve both the record and its caches.
		if err := s.confirmSourceAbsent(ctx, path, make(map[string]map[string]bool)); err != nil {
			return cleaned, err
		}
		if err := s.checkCleanupRoot(ctx, rootInfo); err != nil {
			return cleaned, err
		}
		reportScan(ctx, "cleanup", "remove_cache", path, -1, 0, 0)
		if err := s.removeMissingMediaCaches(ctx, photo, caches); err != nil {
			return cleaned, err
		}
		if err := danmakuCleaner.DeleteMediaFiles(ctx, photo.UUID); err != nil {
			return cleaned, err
		}
		// Cache removal can take time. A restored source keeps its database row
		// (including bookmarks and album memberships); derived caches can rebuild.
		if _, err := scanStat(ctx, path); err == nil {
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return cleaned, err
		}
		if err := s.confirmSourceAbsent(ctx, path, make(map[string]map[string]bool)); err != nil {
			return cleaned, err
		}
		if err := s.checkCleanupRoot(ctx, rootInfo); err != nil {
			return cleaned, err
		}
		// Keep the record on any artifact failure or cancellation so retry can finish.
		if err := repo.HardDeleteMissingPhoto(ctx, photo); err != nil {
			return cleaned, err
		}
		cleaned++
		if progress != nil {
			progress(cleaned, len(missing))
		}
	}
	if err := s.repo.RefreshFolderAlbumCovers(userID); err != nil {
		return cleaned, err
	}
	return cleaned, nil
}

// Confirm absence from a readable directory listing, not just a failed Stat.
// This also protects dangling links and unavailable nested network mounts.
// Each affected parent is listed once, even when thousands of files disappeared.
func (s *PhotoService) confirmSourceAbsent(ctx context.Context, path string, checked map[string]map[string]bool) error {
	// Linked directories may have been remapped or disconnected independently
	// of the library root. Their missing targets are not proof of deletion.
	for parent := filepath.Dir(path); parent != s.sourcePath; parent = filepath.Dir(parent) {
		if !pathWithin(s.sourcePath, parent) {
			return fmt.Errorf("source path is outside library: %s", path)
		}
		info, err := scanIO(ctx, parent, func() (fs.FileInfo, error) { return os.Lstat(parent) })
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("cannot prove deletion through linked or invalid parent: %s", parent)
		}
	}
	child, parent := path, filepath.Dir(path)
	for {
		entries, known := checked[parent]
		if !known {
			info, err := scanStat(ctx, parent)
			if errors.Is(err, fs.ErrNotExist) && parent != s.sourcePath {
				child, parent = parent, filepath.Dir(parent)
				continue
			}
			if err != nil {
				return err
			}
			if !info.IsDir() {
				return fmt.Errorf("source parent is not a directory: %s", parent)
			}
			list, err := readScanDir(ctx, parent)
			if err != nil {
				return err
			}
			entries = make(map[string]bool, len(list))
			for _, entry := range list {
				entries[cleanupEntryName(entry.Name())] = true
			}
			checked[parent] = entries
		}
		if entries[cleanupEntryName(filepath.Base(child))] {
			return fmt.Errorf("source entry still exists but cannot be read: %s", child)
		}
		return ctx.Err()
	}
}

// Preserve ambiguous case/Unicode variants rather than treating a naming
// difference introduced by another OS or SMB server as confirmed deletion.
func cleanupEntryName(name string) string {
	return strings.ToLower(norm.NFC.String(name))
}

func (s *PhotoService) checkCleanupRoot(ctx context.Context, initial fs.FileInfo) error {
	info, err := scanStat(ctx, s.sourcePath)
	if err != nil {
		return err
	}
	if !info.IsDir() || !os.SameFile(initial, info) {
		return fmt.Errorf("library root changed during cleanup: %s", s.sourcePath)
	}
	return ctx.Err()
}

func (s *PhotoService) removeMissingMediaCaches(ctx context.Context, photo *storage.Photo, caches map[string]cleanupCacheDirectory) error {
	paths := make(map[string]string) // artifact path -> owning cache root
	thumbRoot := s.managedThumbnailRoot()
	if err := checkCacheParent(ctx, s.thumbnailPath, filepath.Join(thumbRoot, ".cleanup-check")); err != nil {
		return err
	}
	for _, path := range s.thumbnailCleanupPaths(photo) {
		if path == "" {
			continue
		}
		if pathWithin(thumbRoot, path) {
			paths[path] = thumbRoot
		} else if !s.legacyReadOnly && pathWithin(s.legacyThumbnailRoot, path) {
			paths[path] = s.legacyThumbnailRoot
		}
	}
	// Full-resolution HEIF mappings from earlier conversion generations.
	thumbDir := filepath.Dir(s.ThumbnailPath(photo))
	entries, err := scanIO(ctx, thumbDir, func() ([]fs.DirEntry, error) { return os.ReadDir(thumbDir) })
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), photo.UUID+".heif-") && strings.HasSuffix(entry.Name(), ".jpg") {
			paths[filepath.Join(thumbDir, entry.Name())] = thumbRoot
		}
	}
	var legacy map[string]bool
	for _, directory := range caches {
		if len(directory.legacy) > 0 {
			legacy = legacyMissingMediaCacheNames(photo)
			break
		}
	}
	for root, directory := range caches {
		// UUID-prefixed files include every fingerprint and generation. Hash-only
		// caches predate ownership names and can only be attributed by fingerprint.
		for _, name := range directory.owned[photo.UUID] {
			paths[filepath.Join(root, name)] = root
		}
		for name := range legacy {
			if directory.legacy[name] {
				paths[filepath.Join(root, name)] = root
			}
		}
	}
	for path, root := range paths {
		if err := removeOwnedCacheFile(ctx, root, path); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func legacyMissingMediaCacheNames(photo *storage.Photo) map[string]bool {
	names := make(map[string]bool)
	if photo.MediaKind != storage.MediaKindVideo {
		return names
	}
	relative := photo.SourceRelPath
	if relative == "" {
		relative = photo.StorageRelPath
	}
	relative = strings.ReplaceAll(filepath.ToSlash(relative), `\`, "/")
	for index := -1; index <= 1023; index++ {
		if index >= 0 {
			key := fmt.Sprintf("aac-v1:%s:%s:%d:%d:%d", photo.UUID, relative, photo.Size, photo.SourceModUnix, index)
			names[fmt.Sprintf("%x.mp4", sha256.Sum256([]byte(key)))] = true
		}
		key := fmt.Sprintf("srt-v2-positioned:%s:%s:%d:%d:%d", photo.UUID, photo.SourceRelPath, photo.Size, photo.SourceModUnix, index)
		names[fmt.Sprintf("%x.vtt", sha256.Sum256([]byte(key)))] = true
	}
	return names
}

func pathWithin(root, path string) bool {
	if root == "" || path == "" {
		return false
	}
	relative, err := filepath.Rel(root, path)
	return err == nil && filepath.IsLocal(relative) && relative != "."
}

func removeOwnedCacheFile(ctx context.Context, root, path string) error {
	if err := checkCacheParent(ctx, root, path); err != nil {
		return err
	}
	info, err := scanIO(ctx, path, func() (fs.FileInfo, error) { return os.Lstat(path) })
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("expected a generated cache file, found a directory: %s", path)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func checkCacheParent(ctx context.Context, root, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !pathWithin(root, path) {
		return fmt.Errorf("cache path is outside its root: %s", path)
	}
	// Remove the link itself for a linked file, but never traverse a linked parent
	// into user data outside the configured cache root.
	resolvedRoot, err := scanIO(ctx, root, func() (string, error) { return filepath.EvalSymlinks(root) })
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	parent, err := scanIO(ctx, filepath.Dir(path), func() (string, error) { return filepath.EvalSymlinks(filepath.Dir(path)) })
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if parent != resolvedRoot && !pathWithin(resolvedRoot, parent) {
		return fmt.Errorf("linked cache directory is outside its root: %s", path)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}
