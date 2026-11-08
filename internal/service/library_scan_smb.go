package service

import (
	"context"
	"fmt"
	"hash/fnv"
	"io/fs"
	"path/filepath"
	"strings"

	imgpkg "echogallery/internal/image"
	"echogallery/internal/media"
	"echogallery/internal/storage"
)

type scanMediaEntry struct {
	path  string
	entry fs.DirEntry
}

func isScanMedia(d fs.DirEntry) bool {
	return !strings.HasPrefix(d.Name(), ".") && d.Type().IsRegular() && (imgpkg.SupportedMimeTypes[imgpkg.DetectMimeType(d.Name())] || media.IsSupportedVideoFilename(d.Name()))
}

// A single SMB enumeration supplies both the v2 snapshot and import candidates.
// Stable directories reuse per-file metadata; only entries live for this scan.
func collectSMBLibrarySnapshot(ctx context.Context, root string, previous *storage.LibraryScanSnapshot) (libraryDirectorySnapshot, error) {
	snapshot := libraryDirectorySnapshot{directoryModTimes: map[string]int64{}, directoryEntryHashes: map[string]string{}, directoryMediaCounts: map[string]int{}, directoryMediaHashes: map[string]string{}}
	rootInfo, err := scanStat(ctx, root)
	if err != nil {
		return snapshot, err
	}
	snapshot.rootInfo = rootInfo
	err = walkLibraryDirectories(ctx, root, func(path string) ([]fs.DirEntry, error) { return readScanDir(ctx, path) }, func(path string, d fs.DirEntry, entries []fs.DirEntry) error {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		snapshot.dirCount++
		snapshot.directoryModTimes[rel] = info.ModTime().UnixNano()
		snapshot.directoryEntryHashes[rel] = hashDirectoryEntries(entries)
		stable := previous != nil && previous.Version == libraryScanSnapshotVersion && previous.DirectoryModTimes[rel] == info.ModTime().UnixNano() && libraryDirectoryEntriesMatch(previous, rel, entries)
		count, known := previousDirectoryMediaCount(previous, rel)
		stable = stable && known && previous.DirectoryMediaHashes[rel] != ""
		h := fnv.New64a()
		for _, entry := range entries {
			if !isScanMedia(entry) {
				continue
			}
			filePath := filepath.Join(path, entry.Name())
			snapshot.mediaEntries = append(snapshot.mediaEntries, scanMediaEntry{filePath, entry})
			if stable {
				continue
			}
			fileInfo, err := entry.Info()
			if err != nil {
				return err
			}
			snapshot.directoryMediaCounts[rel]++
			_, _ = fmt.Fprintf(h, "%s\x00%d\x00%d\x00", entry.Name(), fileInfo.Size(), fileInfo.ModTime().UnixNano())
		}
		if stable {
			snapshot.directoryMediaCounts[rel] = count
			snapshot.directoryMediaHashes[rel] = previous.DirectoryMediaHashes[rel]
		} else {
			snapshot.directoryMediaHashes[rel] = fmt.Sprintf("%016x", h.Sum64())
		}
		snapshot.mediaFileCount += snapshot.directoryMediaCounts[rel]
		return nil
	})
	return snapshot, err
}
