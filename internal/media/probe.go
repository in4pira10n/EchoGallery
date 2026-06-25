package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var ErrProbeUnavailable = errors.New("ffprobe 不可用")
var ErrInvalidVideo = errors.New("无效的视频文件")

type VideoMeta struct {
	Width      int       `json:"width"`
	Height     int       `json:"height"`
	DurationMS int64     `json:"duration_ms"`
	FormatName string    `json:"format_name"`
	CodecName  string    `json:"codec_name"`
	FrameRate  float64   `json:"frame_rate,omitempty"`
	TakenAt    time.Time `json:"taken_at,omitempty"`
	Make       string    `json:"make,omitempty"`
	Model      string    `json:"model,omitempty"`
}

type commandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

type ffprobeOutput struct {
	Streams []struct {
		CodecType    string            `json:"codec_type"`
		CodecName    string            `json:"codec_name"`
		Width        int               `json:"width"`
		Height       int               `json:"height"`
		AvgFrameRate string            `json:"avg_frame_rate"`
		RFrameRate   string            `json:"r_frame_rate"`
		Tags         map[string]string `json:"tags"`
		SideDataList []struct {
			Rotation int `json:"rotation"`
		} `json:"side_data_list"`
	} `json:"streams"`
	Format struct {
		FormatName string            `json:"format_name"`
		Duration   string            `json:"duration"`
		Tags       map[string]string `json:"tags"`
	} `json:"format"`
}

func ProbeVideo(path string) (*VideoMeta, error) {
	return probeVideoWithRunner(path, execRunner)
}

func probeVideoWithRunner(path string, runner commandRunner) (*VideoMeta, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := runner(ctx, "ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_streams",
		"-show_format",
		path,
	)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("%w: 请先安装 ffprobe", ErrProbeUnavailable)
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidVideo, err)
	}

	var data ffprobeOutput
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, fmt.Errorf("%w: 解析 ffprobe 输出失败", ErrInvalidVideo)
	}

	meta := &VideoMeta{FormatName: data.Format.FormatName}
	wallClockCreationTime := isWallClockVideoCreationTime(data.Format.Tags)
	for _, stream := range data.Streams {
		if stream.CodecType != "video" {
			continue
		}
		meta.Width = stream.Width
		meta.Height = stream.Height
		if videoStreamRotatedSideways(stream.SideDataList) {
			meta.Width, meta.Height = meta.Height, meta.Width
		}
		meta.CodecName = stream.CodecName
		meta.FrameRate = parseFFprobeFrameRate(stream.AvgFrameRate)
		if meta.FrameRate <= 0 {
			meta.FrameRate = parseFFprobeFrameRate(stream.RFrameRate)
		}
		break
	}

	meta.TakenAt = parseVideoTakenAtTags(data.Format.Tags, wallClockCreationTime)
	meta.Make, meta.Model = parseVideoCameraTags(data.Format.Tags)
	if meta.TakenAt.IsZero() {
		for _, stream := range data.Streams {
			if meta.TakenAt = parseVideoTakenAtTags(stream.Tags, wallClockCreationTime); !meta.TakenAt.IsZero() {
				break
			}
		}
	}
	if meta.Make == "" && meta.Model == "" {
		for _, stream := range data.Streams {
			meta.Make, meta.Model = parseVideoCameraTags(stream.Tags)
			if meta.Make != "" || meta.Model != "" {
				break
			}
		}
	}

	if strings.TrimSpace(data.Format.Duration) != "" {
		d, err := strconv.ParseFloat(data.Format.Duration, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: duration 字段无效", ErrInvalidVideo)
		}
		meta.DurationMS = int64(d * 1000)
	}

	if meta.Width <= 0 || meta.Height <= 0 || meta.DurationMS <= 0 {
		return nil, fmt.Errorf("%w: 缺少必要的视频元数据", ErrInvalidVideo)
	}

	if meta.FormatName == "" {
		meta.FormatName = "mp4"
	}

	return meta, nil
}

func parseFFprobeFrameRate(value string) float64 {
	value = strings.TrimSpace(value)
	if value == "" || value == "0/0" {
		return 0
	}
	if strings.Contains(value, "/") {
		parts := strings.SplitN(value, "/", 2)
		if len(parts) != 2 {
			return 0
		}
		numerator, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		if err != nil {
			return 0
		}
		denominator, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil || denominator == 0 {
			return 0
		}
		return numerator / denominator
	}
	rate, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return rate
}

func videoStreamRotatedSideways(sideData []struct {
	Rotation int `json:"rotation"`
}) bool {
	for _, item := range sideData {
		rotation := item.Rotation % 360
		if rotation < 0 {
			rotation += 360
		}
		if rotation == 90 || rotation == 270 {
			return true
		}
	}
	return false
}

func execRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func parseVideoTakenAtTags(tags map[string]string, wallClockCreationTime bool) time.Time {
	if len(tags) == 0 {
		return time.Time{}
	}
	if ts := parseVideoTakenAt(strings.TrimSpace(tags["com.apple.quicktime.creationdate"])); !ts.IsZero() {
		return ts
	}
	parseTakenAt := parseVideoTakenAt
	if wallClockCreationTime {
		parseTakenAt = parseVideoWallClockTakenAt
	}
	for _, value := range []string{
		strings.TrimSpace(tags["creation_time"]),
		strings.TrimSpace(tags["date"]),
	} {
		if ts := parseTakenAt(value); !ts.IsZero() {
			return ts
		}
	}
	return time.Time{}
}

func parseVideoCameraTags(tags map[string]string) (string, string) {
	if len(tags) == 0 {
		return "", ""
	}
	make := firstVideoTag(tags,
		"com.apple.quicktime.make",
		"com.apple.quicktime.camera.make",
		"make",
		"manufacturer",
		"vendor",
	)
	model := firstVideoTag(tags,
		"com.apple.quicktime.model",
		"com.apple.quicktime.camera.model",
		"model",
		"device_model",
		"camera_model",
	)
	return make, model
}

func firstVideoTag(tags map[string]string, keys ...string) string {
	for _, key := range keys {
		for actual, value := range tags {
			if strings.EqualFold(strings.TrimSpace(actual), key) {
				if text := strings.TrimSpace(value); text != "" {
					return text
				}
			}
		}
	}
	return ""
}

func isWallClockVideoCreationTime(tags map[string]string) bool {
	if len(tags) == 0 {
		return false
	}
	majorBrand := strings.ToUpper(strings.TrimSpace(tags["major_brand"]))
	if majorBrand == "MSNV" {
		return true
	}
	compatibleBrands := strings.ToUpper(strings.TrimSpace(tags["compatible_brands"]))
	return strings.Contains(compatibleBrands, "MSNV")
}

func parseVideoTakenAt(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05-0700",
		"2006-01-02 15:04:05-0700",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		var (
			ts  time.Time
			err error
		)
		if strings.Contains(layout, "-0700") || strings.HasSuffix(layout, "Z07:00") {
			ts, err = time.Parse(layout, value)
		} else {
			ts, err = time.ParseInLocation(layout, value, time.Local)
		}
		if err == nil {
			return ts
		}
	}
	return time.Time{}
}

func parseVideoWallClockTakenAt(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	if stripped, ok := stripVideoTimezoneSuffix(value); ok {
		value = stripped
	}
	layouts := []string{
		"2006-01-02T15:04:05.999999999",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		ts, err := time.ParseInLocation(layout, value, time.Local)
		if err == nil {
			return ts
		}
	}
	return time.Time{}
}

func stripVideoTimezoneSuffix(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	for _, suffix := range []string{"Z"} {
		if strings.HasSuffix(value, suffix) {
			return strings.TrimSuffix(value, suffix), true
		}
	}
	for _, pattern := range []string{"+0000", "-0000"} {
		if strings.HasSuffix(value, pattern) {
			return value[:len(value)-5], true
		}
	}
	if len(value) >= 6 {
		tz := value[len(value)-6:]
		if (tz[0] == '+' || tz[0] == '-') && tz[3] == ':' {
			if isAllDigits(tz[1:3]) && isAllDigits(tz[4:6]) {
				return value[:len(value)-6], true
			}
		}
	}
	if len(value) >= 5 {
		tz := value[len(value)-5:]
		if (tz[0] == '+' || tz[0] == '-') && isAllDigits(tz[1:5]) {
			return value[:len(value)-5], true
		}
	}
	return "", false
}

func isAllDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
