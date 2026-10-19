package danmaku

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func Root(libraryPath string) string { return filepath.Join(libraryPath, ".echogallery", "danmaku") }

func safeBVID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, `/\\`) {
		return "unknown"
	}
	return value
}

func WritePage(root string, bvid string, page Page, data []byte, overwrite bool) (bool, error) {
	dir := filepath.Join(root, safeBVID(bvid))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return false, err
	}
	target := filepath.Join(dir, filepath.Base(page.File))
	if !overwrite {
		if info, err := os.Stat(target); err == nil && info.Size() > 0 {
			return true, nil
		}
	}
	tmp := target + ".part"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return false, err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return false, err
	}
	return false, nil
}

func WriteMetadata(root string, metadata Metadata) error {
	dir := filepath.Join(root, safeBVID(metadata.BVID))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp := filepath.Join(dir, "metadata.json.part")
	target := filepath.Join(dir, "metadata.json")
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func ListMetadata(root string) ([]Metadata, error) {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return []Metadata{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]Metadata, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(root, entry.Name(), "metadata.json"))
		if readErr != nil {
			continue
		}
		var metadata Metadata
		if json.Unmarshal(data, &metadata) == nil {
			result = append(result, metadata)
		}
	}
	return result, nil
}

func ReadMetadata(root, bvid string) (Metadata, error) {
	data, err := os.ReadFile(MetadataPath(root, bvid))
	if err != nil {
		return Metadata{}, err
	}
	var metadata Metadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return Metadata{}, err
	}
	return metadata, nil
}

func MetadataPath(root, bvid string) string {
	return filepath.Join(root, safeBVID(bvid), "metadata.json")
}

func NewMetadata(video Video, mediaID int64, mediaUUID string) Metadata {
	return Metadata{Video: video, MediaID: mediaID, MediaUUID: mediaUUID, DownloadedAt: time.Now()}
}

func ValidateRoot(root string) error {
	if strings.TrimSpace(root) == "" {
		return fmt.Errorf("danmaku root is empty")
	}
	return os.MkdirAll(root, 0755)
}
