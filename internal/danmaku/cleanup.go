package danmaku

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Managers may belong to different runtime sessions for the same library.
var mediaCleanupGate = struct {
	sync.Mutex
	active map[string]int
}{active: make(map[string]int)}

// MediaFileCleaner indexes matches once for a potentially large batch cleanup.
type MediaFileCleaner struct {
	root    string
	matches map[string][]string
}

func NewMediaFileCleaner(root string) (*MediaFileCleaner, error) {
	cleaner := &MediaFileCleaner{root: root, matches: make(map[string][]string)}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return cleaner, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, entry.Name(), "metadata.json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var item Metadata
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		if item.MediaUUID == "" {
			continue
		}
		bvid, _, err := ParseBVID(item.BVID)
		if err != nil || bvid != item.BVID || bvid != entry.Name() {
			return nil, fmt.Errorf("invalid danmaku cache identity: %s", item.BVID)
		}
		cleaner.matches[item.MediaUUID] = append(cleaner.matches[item.MediaUUID], bvid)
	}
	return cleaner, nil
}

// DeleteMediaFiles removes only records still explicitly matched to this media.
// Other matches and unassociated downloads remain intact. Downloads finish first.
func (c *MediaFileCleaner) DeleteMediaFiles(ctx context.Context, mediaUUID string) error {
	mediaCleanupGate.Lock()
	defer mediaCleanupGate.Unlock()
	if mediaCleanupGate.active[filepath.Clean(c.root)] > 0 {
		return ErrTaskRunning
	}
	for _, bvid := range c.matches[mediaUUID] {
		if err := ctx.Err(); err != nil {
			return err
		}
		item, err := ReadMetadata(c.root, bvid)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if mediaUUID == "" || item.MediaUUID != mediaUUID {
			continue
		}
		if err := os.RemoveAll(filepath.Join(c.root, bvid)); err != nil {
			return err
		}
	}
	return ctx.Err()
}
