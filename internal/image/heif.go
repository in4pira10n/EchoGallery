package image

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var ErrHEIFProbeUnavailable = errors.New("ffprobe unavailable for HEIF metadata")
var ErrHEIFConverterUnavailable = errors.New("ffmpeg unavailable for HEIF conversion")

type heifMetadata struct {
	Width       int
	Height      int
	Orientation int
	TakenAt     time.Time
	Make        string
	Model       string
}

type heifTile struct {
	StreamIndex      int `json:"stream_index"`
	HorizontalOffset int `json:"tile_horizontal_offset"`
	VerticalOffset   int `json:"tile_vertical_offset"`
}

type heifTileGridComponent struct {
	HorizontalOffset int        `json:"horizontal_offset"`
	VerticalOffset   int        `json:"vertical_offset"`
	Width            int        `json:"width"`
	Height           int        `json:"height"`
	Subcomponents    []heifTile `json:"subcomponents"`
}

type heifProbeOutput struct {
	Streams []struct {
		Width        int               `json:"width"`
		Height       int               `json:"height"`
		Tags         map[string]string `json:"tags"`
		SideDataList []struct {
			Rotation int `json:"rotation"`
		} `json:"side_data_list"`
	} `json:"streams"`
	StreamGroups []struct {
		Type       string                  `json:"type"`
		Components []heifTileGridComponent `json:"components"`
	} `json:"stream_groups"`
	Format struct {
		Tags map[string]string `json:"tags"`
	} `json:"format"`
}

type heifCommandRunner func(context.Context, string, ...string) ([]byte, error)

func extractHEIFMetaFromPath(path, filename string, fallbackTime time.Time) (*Meta, error) {
	return extractHEIFMetaFromPathContext(context.Background(), path, filename, fallbackTime)
}

func extractHEIFMetaFromPathContext(ctx context.Context, path, filename string, fallbackTime time.Time) (*Meta, error) {
	if !SupportedMimeTypes[DetectMimeType(filename)] {
		return nil, fmt.Errorf("不支持的图片格式: %s", DetectMimeType(filename))
	}
	probed, err := probeHEIFContext(ctx, path, runHEIFCommand)
	if err != nil {
		return nil, err
	}
	contents, readErr := readFileContext(ctx, path)
	var exifData *EXIFData
	var exifTakenAt time.Time
	if readErr == nil {
		exifData, exifTakenAt, _ = parseHEIFEXIFContext(ctx, contents, true)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	takenAt := exifTakenAt
	if takenAt.IsZero() {
		takenAt = probed.TakenAt
	}
	if takenAt.IsZero() {
		takenAt = fallbackTime
	}
	orientation := probed.Orientation
	makeName, modelName := probed.Make, probed.Model
	if exifData == nil {
		exifData = &EXIFData{}
	}
	if exifData.Orientation != 0 {
		orientation = exifData.Orientation
	}
	if exifData.Make == "" {
		exifData.Make = makeName
	}
	if exifData.Model == "" {
		exifData.Model = modelName
	}
	width, height := orientedDimensions(probed.Width, probed.Height, orientation)
	exifData.Orientation = orientation
	exifData.TakenAt = takenAt
	exifData.Width = width
	exifData.Height = height
	return &Meta{
		Width:    width,
		Height:   height,
		MimeType: DetectMimeType(filename),
		TakenAt:  takenAt,
		EXIF:     exifData,
	}, nil
}

func probeHEIF(path string, runner heifCommandRunner) (*heifMetadata, error) {
	return probeHEIFContext(context.Background(), path, runner)
}

func probeHEIFContext(parent context.Context, path string, runner heifCommandRunner) (*heifMetadata, error) {
	data, err := readHEIFProbeContext(parent, path, runner, 15*time.Second)
	if err != nil {
		return nil, err
	}
	if len(data.Streams) == 0 || data.Streams[0].Width <= 0 || data.Streams[0].Height <= 0 {
		return nil, errors.New("HEIF 文件中没有可读取的图片")
	}
	stream := data.Streams[0]
	tags := make(map[string]string, len(data.Format.Tags)+len(stream.Tags))
	for key, value := range data.Format.Tags {
		tags[key] = value
	}
	for key, value := range stream.Tags {
		tags[key] = value
	}
	orientation := 0
	for _, sideData := range stream.SideDataList {
		rotation := sideData.Rotation % 360
		if rotation < 0 {
			rotation += 360
		}
		switch rotation {
		case 90:
			orientation = 6
		case 180:
			orientation = 3
		case 270:
			orientation = 8
		}
		if orientation != 0 {
			break
		}
	}
	width, height := stream.Width, stream.Height
	if grid := largestHEIFGridComponent(data); grid != nil {
		width, height = grid.Width, grid.Height
	}
	return &heifMetadata{
		Width:       width,
		Height:      height,
		Orientation: orientation,
		TakenAt:     heifTakenAt(tags),
		Make:        firstHEIFTag(tags, "make", "manufacturer", "com.apple.quicktime.make"),
		Model:       firstHEIFTag(tags, "model", "device_model", "camera_model", "com.apple.quicktime.model"),
	}, nil
}

func readHEIFProbe(path string, runner heifCommandRunner, timeout time.Duration) (*heifProbeOutput, error) {
	return readHEIFProbeContext(context.Background(), path, runner, timeout)
}

func readHEIFProbeContext(parent context.Context, path string, runner heifCommandRunner, timeout time.Duration) (*heifProbeOutput, error) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	out, err := runner(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_streams",
		"-show_stream_groups",
		"-show_format",
		"-of", "json",
		path,
	)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("%w: 请安装包含 ffprobe 的 FFmpeg", ErrHEIFProbeUnavailable)
		}
		return nil, fmt.Errorf("读取 HEIF 图片信息失败: %w%s", err, heifCommandOutputSuffix(out))
	}
	var data heifProbeOutput
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, fmt.Errorf("解析 HEIF 图片信息失败: %w", err)
	}
	return &data, nil
}

func largestHEIFGridComponent(data *heifProbeOutput) *heifTileGridComponent {
	if data == nil {
		return nil
	}
	var largest *heifTileGridComponent
	for _, group := range data.StreamGroups {
		if !strings.EqualFold(strings.TrimSpace(group.Type), "Tile Grid") {
			continue
		}
		for i := range group.Components {
			component := &group.Components[i]
			if component.Width <= 0 || component.Height <= 0 || len(component.Subcomponents) == 0 {
				continue
			}
			if largest == nil || int64(component.Width)*int64(component.Height) > int64(largest.Width)*int64(largest.Height) {
				largest = component
			}
		}
	}
	return largest
}

func heifTakenAt(tags map[string]string) time.Time {
	for _, key := range []string{"DateTimeOriginal", "date_time_original", "creation_time", "date", "DateTimeDigitized", "DateTime"} {
		value := firstHEIFTag(tags, key)
		if value == "" {
			continue
		}
		for _, layout := range []string{
			time.RFC3339Nano,
			"2006-01-02T15:04:05-0700",
			"2006-01-02 15:04:05-0700",
			"2006:01:02 15:04:05",
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
		} {
			var (
				parsed time.Time
				err    error
			)
			if strings.Contains(layout, "-0700") || strings.Contains(layout, "Z07:00") {
				parsed, err = time.Parse(layout, value)
			} else {
				parsed, err = time.ParseInLocation(layout, value, time.Local)
			}
			if err == nil {
				return parsed
			}
		}
	}
	return time.Time{}
}

func firstHEIFTag(tags map[string]string, keys ...string) string {
	for _, key := range keys {
		for actual, value := range tags {
			if strings.EqualFold(strings.TrimSpace(actual), key) {
				if value = strings.TrimSpace(value); value != "" {
					return value
				}
			}
		}
	}
	return ""
}

// ConvertHEIFToJPEG writes a full-resolution JPEG without changing the source.
// The output is first written beside the destination and renamed into place so
// readers never see a partially converted preview.
func ConvertHEIFToJPEG(inputPath, outputPath string) error {
	return ConvertHEIFToJPEGContext(context.Background(), inputPath, outputPath)
}

func ConvertHEIFToJPEGContext(ctx context.Context, inputPath, outputPath string) error {
	return convertHEIFToJPEGWithRunnerContext(ctx, inputPath, outputPath, runHEIFCommand)
}

func convertHEIFToJPEGWithRunner(inputPath, outputPath string, runner heifCommandRunner) error {
	return convertHEIFToJPEGWithRunnerContext(context.Background(), inputPath, outputPath, runner)
}

func convertHEIFToJPEGWithRunnerContext(parent context.Context, inputPath, outputPath string, runner heifCommandRunner) error {
	if err := parent.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(inputPath) == "" || strings.TrimSpace(outputPath) == "" {
		return errors.New("HEIF 转换路径为空")
	}
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建 HEIF 预览目录失败: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".heif-preview-*.jpg")
	if err != nil {
		return fmt.Errorf("创建 HEIF 预览临时文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("关闭 HEIF 临时文件失败: %w", err)
	}
	defer os.Remove(tmpPath)

	probe, err := readHEIFProbeContext(parent, inputPath, runner, 30*time.Second)
	if err != nil {
		return err
	}
	filterGraph, mappedStream, err := heifConversionSelection(probe)
	if err != nil {
		return err
	}
	orientation := heifProbeOrientation(probe)
	if orientation == 0 {
		if contents, readErr := os.ReadFile(inputPath); readErr == nil {
			if exifData, _, parseErr := parseHEIFEXIF(contents, false); parseErr == nil && exifData != nil {
				orientation = exifData.Orientation
			}
		}
	}
	orientationFilter := heifOrientationFilter(orientation)
	args := []string{"-y", "-hide_banner", "-loglevel", "error", "-i", inputPath}
	if filterGraph != "" {
		if orientationFilter != "" {
			filterGraph, mappedStream = appendHEIFOrientationFilter(filterGraph, mappedStream, orientationFilter)
		}
		args = append(args, "-filter_complex", filterGraph)
	} else if orientationFilter != "" {
		args = append(args, "-vf", orientationFilter)
	}
	args = append(args,
		"-map", mappedStream,
		"-frames:v", "1",
		"-q:v", "2",
		"-f", "image2",
		"-update", "1",
		tmpPath,
	)
	ctx, cancel := context.WithTimeout(parent, 30*time.Minute)
	defer cancel()
	out, err := runner(ctx, "ffmpeg", args...)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("%w: 请先安装 FFmpeg", ErrHEIFConverterUnavailable)
		}
		return fmt.Errorf("HEIF 转换为 JPEG 失败: %w%s", err, heifCommandOutputSuffix(out))
	}
	file, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("FFmpeg 未生成 HEIF 预览: %w", err)
	}
	_, decodeErr := jpeg.DecodeConfig(file)
	_ = file.Close()
	if decodeErr != nil {
		return fmt.Errorf("FFmpeg 生成的 HEIF 预览不是有效 JPEG: %w", decodeErr)
	}
	if err := os.Rename(tmpPath, outputPath); err != nil {
		return fmt.Errorf("保存 HEIF 预览缓存失败: %w", err)
	}
	return nil
}

func heifProbeOrientation(data *heifProbeOutput) int {
	if data == nil || len(data.Streams) == 0 {
		return 0
	}
	for _, sideData := range data.Streams[0].SideDataList {
		rotation := sideData.Rotation % 360
		if rotation < 0 {
			rotation += 360
		}
		switch rotation {
		case 90:
			return 6
		case 180:
			return 3
		case 270:
			return 8
		}
	}
	return 0
}

// heifOrientationFilter normalizes the derived JPEG. The source HEIC is never
// rewritten; only the browser-friendly cache receives this presentation transform.
func heifOrientationFilter(orientation int) string {
	switch orientation {
	case 2:
		return "hflip"
	case 3:
		return "hflip,vflip"
	case 4:
		return "vflip"
	case 5:
		return "hflip,transpose=cclock"
	case 6:
		return "transpose=clock"
	case 7:
		return "hflip,transpose=clock"
	case 8:
		return "transpose=cclock"
	default:
		return ""
	}
}

func appendHEIFOrientationFilter(graph, mappedStream, filter string) (string, string) {
	const output = "[heif_preview]"
	const oriented = "[heif_oriented]"
	if !strings.HasSuffix(graph, output) {
		return graph, mappedStream
	}
	graph = strings.TrimSuffix(graph, output) + oriented + ";" + oriented + filter + output
	return graph, output
}

func heifConversionSelection(data *heifProbeOutput) (filterGraph, mappedStream string, err error) {
	grid := largestHEIFGridComponent(data)
	if grid == nil {
		return "", "0:v:0", nil
	}
	if grid.Width <= 0 || grid.Height <= 0 || grid.HorizontalOffset < 0 || grid.VerticalOffset < 0 {
		return "", "", errors.New("HEIF 网格图片尺寸或偏移无效")
	}
	if len(grid.Subcomponents) == 1 {
		tile := grid.Subcomponents[0]
		if tile.StreamIndex < 0 {
			return "", "", errors.New("HEIF 网格图片包含无效的图块索引")
		}
		return fmt.Sprintf("[0:%d]crop=%d:%d:%d:%d[heif_preview]", tile.StreamIndex, grid.Width, grid.Height, grid.HorizontalOffset, grid.VerticalOffset), "[heif_preview]", nil
	}
	if len(grid.Subcomponents) > 256 {
		return "", "", fmt.Errorf("HEIF 网格图块数量过多: %d", len(grid.Subcomponents))
	}
	inputs := make([]string, 0, len(grid.Subcomponents))
	positions := make([]string, 0, len(grid.Subcomponents))
	for _, tile := range grid.Subcomponents {
		if tile.StreamIndex < 0 || tile.HorizontalOffset < 0 || tile.VerticalOffset < 0 {
			return "", "", errors.New("HEIF 网格图片包含无效的图块索引或位置")
		}
		inputs = append(inputs, fmt.Sprintf("[0:%d]", tile.StreamIndex))
		positions = append(positions, fmt.Sprintf("%d_%d", tile.HorizontalOffset, tile.VerticalOffset))
	}
	graph := fmt.Sprintf("%sxstack=inputs=%d:layout=%s:shortest=1,crop=%d:%d:%d:%d[heif_preview]",
		strings.Join(inputs, ""), len(inputs), strings.Join(positions, "|"),
		grid.Width, grid.Height, grid.HorizontalOffset, grid.VerticalOffset)
	return graph, "[heif_preview]", nil
}

func runHEIFCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func readFileContext(ctx context.Context, path string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(contextReader{ctx: ctx, reader: file})
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}

func heifCommandOutputSuffix(out []byte) string {
	message := strings.TrimSpace(string(out))
	if message == "" {
		return ""
	}
	const maxLen = 1200
	if len(message) > maxLen {
		message = "..." + message[len(message)-maxLen:]
	}
	return ": " + message
}
