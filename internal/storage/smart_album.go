package storage

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// NormalizeSmartAlbumDefinition validates and normalizes a smart album's rules.
func NormalizeSmartAlbumDefinition(matchMode string, rules []SmartAlbumRule) (string, []SmartAlbumRule, error) {
	matchMode = strings.ToLower(strings.TrimSpace(matchMode))
	if matchMode != "all" && matchMode != "any" {
		return "", nil, fmt.Errorf("匹配方式必须是“全部”或“任意”")
	}
	if len(rules) == 0 || len(rules) > 10 {
		return "", nil, fmt.Errorf("智能相册需要 1 到 10 条匹配规则")
	}

	normalized := make([]SmartAlbumRule, len(rules))
	for i, rule := range rules {
		rule.Field = strings.ToLower(strings.TrimSpace(rule.Field))
		rule.Operator = strings.ToLower(strings.TrimSpace(rule.Operator))
		rule.Value = strings.TrimSpace(rule.Value)
		rule.Min = strings.TrimSpace(rule.Min)
		rule.Max = strings.TrimSpace(rule.Max)

		switch rule.Field {
		case "media_type":
			if rule.Operator != "is" && rule.Operator != "is_not" {
				return "", nil, fmt.Errorf("第 %d 条媒体类型规则的条件无效", i+1)
			}
			rule.Value = strings.ToLower(rule.Value)
			if rule.Value != MediaKindImage && rule.Value != MediaKindVideo {
				return "", nil, fmt.Errorf("第 %d 条规则请选择图片或视频", i+1)
			}
			rule.Min, rule.Max = "", ""
		case "duration":
			if rule.Operator != "between" && rule.Operator != "at_least" && rule.Operator != "at_most" {
				return "", nil, fmt.Errorf("第 %d 条时长规则的条件无效", i+1)
			}
			switch rule.Operator {
			case "between":
				min, err := ParseSmartAlbumDuration(rule.Min)
				if err != nil {
					return "", nil, fmt.Errorf("第 %d 条规则的最短时长无效", i+1)
				}
				max, err := ParseSmartAlbumDuration(rule.Max)
				if err != nil || max < min {
					return "", nil, fmt.Errorf("第 %d 条规则的最长时长无效或小于最短时长", i+1)
				}
			case "at_least":
				if _, err := ParseSmartAlbumDuration(rule.Min); err != nil {
					return "", nil, fmt.Errorf("第 %d 条规则请输入有效的最短时长", i+1)
				}
				rule.Max = ""
			case "at_most":
				if _, err := ParseSmartAlbumDuration(rule.Max); err != nil {
					return "", nil, fmt.Errorf("第 %d 条规则请输入有效的最长时长", i+1)
				}
				rule.Min = ""
			}
			rule.Value = ""
		case "filename":
			switch rule.Operator {
			case "contains", "not_contains", "starts_with", "ends_with":
			default:
				return "", nil, fmt.Errorf("第 %d 条文件名规则的条件无效", i+1)
			}
			if rule.Value == "" || len([]rune(rule.Value)) > 80 {
				return "", nil, fmt.Errorf("第 %d 条规则请输入 1 到 80 个字符", i+1)
			}
			rule.Min, rule.Max = "", ""
		default:
			return "", nil, fmt.Errorf("第 %d 条规则的字段无效", i+1)
		}
		normalized[i] = rule
	}
	return matchMode, normalized, nil
}

// DescribeSmartAlbumRules creates the default human-readable album description.
func DescribeSmartAlbumRules(rules []SmartAlbumRule) string {
	descriptions := make([]string, 0, len(rules))
	for _, rule := range rules {
		switch rule.Field {
		case "media_type":
			mediaType := "图片"
			if rule.Value == MediaKindVideo {
				mediaType = "视频"
			}
			verb := "是"
			if rule.Operator == "is_not" {
				verb = "不是"
			}
			descriptions = append(descriptions, "媒体类型"+verb+mediaType)
		case "duration":
			switch rule.Operator {
			case "between":
				descriptions = append(descriptions, fmt.Sprintf("视频时长介于 %s 至 %s", describeSmartAlbumDuration(rule.Min), describeSmartAlbumDuration(rule.Max)))
			case "at_most":
				descriptions = append(descriptions, "视频时长小于 "+describeSmartAlbumDuration(rule.Max))
			default:
				descriptions = append(descriptions, "视频时长大于 "+describeSmartAlbumDuration(rule.Min))
			}
		case "filename":
			operator := map[string]string{
				"contains":     "包含",
				"not_contains": "不包含",
				"starts_with":  "开头为",
				"ends_with":    "结尾为",
			}[rule.Operator]
			if operator == "" {
				operator = "包含"
			}
			value := strings.TrimSpace(rule.Value)
			if value != "" {
				value = " " + value
			}
			descriptions = append(descriptions, "文件名"+operator+value)
		}
	}
	return strings.Join(descriptions, " · ")
}

func describeSmartAlbumDuration(value string) string {
	milliseconds, err := ParseSmartAlbumDuration(value)
	if err != nil {
		return strings.TrimSpace(value)
	}
	totalSeconds := milliseconds / 1000
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	if hours > 0 {
		result := fmt.Sprintf("%d 小时", hours)
		if minutes > 0 {
			result += fmt.Sprintf(" %d 分钟", minutes)
		}
		if seconds > 0 {
			result += fmt.Sprintf(" %d 秒", seconds)
		}
		return result
	}
	if minutes > 0 {
		if seconds > 0 {
			return fmt.Sprintf("%d 分钟 %d 秒", minutes, seconds)
		}
		return fmt.Sprintf("%d 分钟", minutes)
	}
	return fmt.Sprintf("%d 秒", seconds)
}

// ParseSmartAlbumDuration parses the editor's M:SS or H:MM:SS duration format.
func ParseSmartAlbumDuration(value string) (int64, error) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) == 0 || len(parts) > 3 {
		return 0, fmt.Errorf("invalid duration")
	}
	values := make([]int64, len(parts))
	for i, part := range parts {
		if part == "" || strings.HasPrefix(part, "-") {
			return 0, fmt.Errorf("invalid duration")
		}
		number, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration")
		}
		values[i] = number
	}
	if len(values) > 1 {
		for _, component := range values[1:] {
			if component > 59 {
				return 0, fmt.Errorf("invalid duration")
			}
		}
	}

	units := make([]int64, len(values))
	switch len(values) {
	case 1:
		units[0] = 60_000
	case 2:
		units[0], units[1] = 60_000, 1_000
	case 3:
		units[0], units[1], units[2] = 3_600_000, 60_000, 1_000
	}
	var total int64
	for i, number := range values {
		if number > (math.MaxInt64-total)/units[i] {
			return 0, fmt.Errorf("duration too large")
		}
		total += number * units[i]
	}
	return total, nil
}
