package api

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"echogallery/internal/config"
)

type playerKeymapResponse struct {
	Content string `json:"content"`
}

func handleGetPlayerKeymap(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := requestConfig(c, cfg)
		profileKeymap := ""
		if cfg != nil {
			profileKeymap = cfg.Preferences.PlayerKeymap
		}
		c.JSON(200, playerKeymapResponse{Content: effectivePlayerKeymapContent(profileKeymap)})
	}
}

func playerKeymapFileCandidates() []string {
	candidates := []string{"ZXCWASD.conf"}
	if exe, err := os.Executable(); err == nil {
		exePath := filepath.Join(filepath.Dir(exe), "ZXCWASD.conf")
		if exePath != candidates[0] {
			candidates = append(candidates, exePath)
		}
	}
	return candidates
}

func loadPlayerKeymapContent() (string, error) {
	for _, path := range playerKeymapFileCandidates() {
		data, err := os.ReadFile(path)
		if err == nil {
			return normalizePlayerKeymapContent(string(data)), nil
		}
	}
	return "", os.ErrNotExist
}

func effectivePlayerKeymapContent(profileKeymap string) string {
	if content, err := loadPlayerKeymapContent(); err == nil && strings.TrimSpace(content) != "" {
		return content
	}
	if content := normalizePlayerKeymapContent(profileKeymap); content != "" {
		return content
	}
	return defaultPlayerKeymapContent()
}

func savePlayerKeymapContent(content string) error {
	normalized := normalizePlayerKeymapContent(content)
	target := ""
	for _, path := range playerKeymapFileCandidates() {
		if _, err := os.Stat(path); err == nil {
			target = path
			break
		}
	}
	if target == "" {
		candidates := playerKeymapFileCandidates()
		target = candidates[len(candidates)-1]
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(normalized), 0644)
}
