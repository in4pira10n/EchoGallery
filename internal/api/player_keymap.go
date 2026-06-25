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
		content, err := loadPlayerKeymapContent()
		if err != nil {
			if cfg != nil && cfg.Preferences.PlayerKeymap != "" {
				c.JSON(200, playerKeymapResponse{Content: cfg.Preferences.PlayerKeymap})
				return
			}
			c.JSON(200, playerKeymapResponse{Content: ""})
			return
		}
		c.JSON(200, playerKeymapResponse{Content: content})
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
			return strings.ReplaceAll(string(data), "\r\n", "\n"), nil
		}
	}
	return "", os.ErrNotExist
}

func savePlayerKeymapContent(content string) error {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
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
