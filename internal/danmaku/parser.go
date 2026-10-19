package danmaku

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var bvidPattern = regexp.MustCompile(`(?i)\bBV[0-9A-Za-z]{10}\b`)

func ParseBVID(value string) (string, int, error) {
	input := strings.TrimSpace(value)
	if input == "" {
		return "", 0, fmt.Errorf("empty Bilibili URL or BV id")
	}
	match := bvidPattern.FindString(input)
	if match == "" {
		return "", 0, fmt.Errorf("could not find a valid BV id")
	}
	page := 0
	if parsed, err := url.Parse(input); err == nil {
		if raw := parsed.Query().Get("p"); raw != "" {
			page, _ = strconv.Atoi(raw)
			if page < 1 {
				page = 0
			}
		}
	}
	return match, page, nil
}
