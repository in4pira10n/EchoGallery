package api

import "testing"

func TestDefaultPlayerKeymapUsesZXCWASDLayout(t *testing.T) {
	content := defaultPlayerKeymapContent()
	for _, binding := range []string{
		"x cycle pause",
		"z seek -5",
		"c seek 5",
		"w add volume 5",
		"s add volume -5",
		"RIGHT cycle danmaku",
	} {
		if !containsLine(content, binding) {
			t.Fatalf("default keymap is missing %q", binding)
		}
	}
}

func containsLine(content, expected string) bool {
	for _, line := range splitLines(content) {
		if line == expected {
			return true
		}
	}
	return false
}

func splitLines(content string) []string {
	var lines []string
	for len(content) > 0 {
		line := content
		if index := indexByte(content, '\n'); index >= 0 {
			line = content[:index]
			content = content[index+1:]
		} else {
			content = ""
		}
		lines = append(lines, line)
	}
	return lines
}

func indexByte(value string, target byte) int {
	for index := 0; index < len(value); index++ {
		if value[index] == target {
			return index
		}
	}
	return -1
}
