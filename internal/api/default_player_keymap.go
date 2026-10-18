package api

import (
	_ "embed"
	"strings"
)

//go:embed default_player_keymap.conf
var embeddedDefaultPlayerKeymap string

func normalizePlayerKeymapContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	return content + "\n"
}

func defaultPlayerKeymapContent() string {
	return normalizePlayerKeymapContent(embeddedDefaultPlayerKeymap)
}
