package config

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// existingPortableLibraryID reads an identity only from an existing library
// database. Older portable libraries did not store the ID, so their thumbnail
// directory is used as a fallback.
type portableLibraryMetadata struct {
	ID          string
	Name        string
	AccentColor string
}

func existingPortableLibraryMetadata(storagePath string) (portableLibraryMetadata, error) {
	var metadata portableLibraryMetadata
	root := LibraryDataRoot(storagePath)
	dbPath := filepath.Join(root, "metadata.sqlite")
	if info, err := os.Stat(dbPath); err != nil || info.IsDir() {
		return metadata, nil
	}
	db, err := sql.Open("sqlite", sqliteReadOnlyURI(dbPath))
	if err != nil {
		return metadata, err
	}
	defer db.Close()
	if err := db.QueryRow(`SELECT value FROM app_meta WHERE key = 'library_id'`).Scan(&metadata.ID); err == nil {
		metadata.ID = normalizeLibraryID(metadata.ID)
	}
	_ = db.QueryRow(`SELECT value FROM app_meta WHERE key = 'library_name'`).Scan(&metadata.Name)
	_ = db.QueryRow(`SELECT value FROM app_meta WHERE key = 'library_accent_color'`).Scan(&metadata.AccentColor)
	if metadata.ID != "" {
		return metadata, nil
	}
	entries, err := os.ReadDir(filepath.Join(root, "thumbnails"))
	if err != nil {
		if os.IsNotExist(err) {
			return metadata, nil
		}
		return metadata, err
	}
	var candidates []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || len(name) <= 4 || normalizeLibraryID(name) != name {
			continue
		}
		candidates = append(candidates, name)
	}
	if len(candidates) == 1 {
		metadata.ID = candidates[0]
		return metadata, nil
	}
	if len(candidates) == 0 {
		return metadata, nil
	}
	rows, err := db.Query(`SELECT uuid FROM photos WHERE deleted_at IS NULL`)
	if err != nil {
		return metadata, fmt.Errorf("无法核对已有资源库的缩略图目录: %w", err)
	}
	defer rows.Close()
	scores := make(map[string]int, len(candidates))
	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			return metadata, err
		}
		key := strings.ReplaceAll(strings.ToLower(uuid), "-", "")
		if len(key) < 4 {
			continue
		}
		for _, candidate := range candidates {
			path := filepath.Join(root, "thumbnails", candidate, key[:2], key[2:4], uuid+".webp")
			if _, err := os.Stat(path); err == nil {
				scores[candidate]++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return metadata, err
	}
	best, matches := "", 0
	for _, candidate := range candidates {
		if scores[candidate] > 0 {
			best = candidate
			matches++
		}
	}
	if matches != 1 {
		return metadata, fmt.Errorf("资源库 %s 存在多个缩略图 ID 目录，无法唯一识别", storagePath)
	}
	metadata.ID = best
	return metadata, nil
}

func existingPortableLibraryID(storagePath string) (string, error) {
	metadata, err := existingPortableLibraryMetadata(storagePath)
	return metadata.ID, err
}

func PortableLibraryMetadataMatches(storagePath, id, name, accentColor string) bool {
	metadata, err := existingPortableLibraryMetadata(storagePath)
	if err != nil || metadata.ID == "" {
		return false
	}
	return metadata.ID == normalizeLibraryID(id) &&
		strings.TrimSpace(metadata.Name) == strings.TrimSpace(name) &&
		strings.TrimSpace(metadata.AccentColor) == strings.TrimSpace(accentColor)
}

func ValidatePortableLibraryIdentity(storagePath, expectedID string) error {
	existing, err := existingPortableLibraryID(storagePath)
	if err != nil {
		return err
	}
	if existing != "" && existing != normalizeLibraryID(expectedID) {
		return fmt.Errorf("资源库 ID 不一致：目录记录为 %s，当前配置为 %s", existing, expectedID)
	}
	return nil
}
