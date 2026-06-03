package image

import (
	"crypto/sha1"
	"encoding/hex"
	"path/filepath"
	"strings"
)

func ThumbnailFlatPath(root, id string) string {
	return filepath.Join(root, id+".webp")
}

func ThumbnailShardPath(root, id string) string {
	first, second := thumbnailShardSegments(id)
	return filepath.Join(root, first, second, id+".webp")
}

func thumbnailShardSegments(id string) (string, string) {
	normalized := normalizeThumbnailShardKey(id)
	if len(normalized) < 4 {
		sum := sha1.Sum([]byte(id))
		normalized += hex.EncodeToString(sum[:])
	}
	return normalized[:2], normalized[2:4]
}

func normalizeThumbnailShardKey(id string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(id)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
