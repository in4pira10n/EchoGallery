package image

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rwcarlsen/goexif/exif"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
)

// SupportedMimeTypes 支持的图片 MIME 类型
var SupportedMimeTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
	"image/bmp":  true,
	"image/tiff": true,
}

// Meta 图片元数据
type Meta struct {
	Width    int
	Height   int
	MimeType string
	TakenAt  time.Time // 拍摄时间
	EXIF     *EXIFData
}

// EXIFData 从图片中提取的 EXIF 信息
type EXIFData struct {
	Make         string    `json:"make,omitempty"`
	Model        string    `json:"model,omitempty"`
	Orientation  int       `json:"orientation,omitempty"`
	TakenAt      time.Time `json:"taken_at,omitempty"`
	Width        int       `json:"width,omitempty"`
	Height       int       `json:"height,omitempty"`
	FNumber      string    `json:"f_number,omitempty"`
	ExposureTime string    `json:"exposure_time,omitempty"`
	ISOSpeed     int       `json:"iso_speed,omitempty"`
	FocalLength  string    `json:"focal_length,omitempty"`
	Latitude     float64   `json:"latitude,omitempty"`
	Longitude    float64   `json:"longitude,omitempty"`
	HasGPS       bool      `json:"has_gps,omitempty"`
	Location     string    `json:"location_address,omitempty"`
}

// ExtractMeta 从 ReadSeeker 中提取图片元数据
// fileCreateTime 作为没有 EXIF 时的后备时间
func ExtractMeta(rs io.ReadSeeker, filename string, fileCreateTime time.Time) (*Meta, error) {
	mimeType := DetectMimeType(filename)
	if !SupportedMimeTypes[mimeType] {
		return nil, fmt.Errorf("不支持的图片格式: %s", mimeType)
	}

	// 尝试读取 EXIF
	var exifData *EXIFData
	takenAt := fileCreateTime

	if mimeType == "image/jpeg" {
		if ed, t, err := parseEXIF(rs); err == nil {
			exifData = ed
			if !t.IsZero() {
				takenAt = t
			}
		}
		// 重置读取位置
		if _, err := rs.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("seek 失败: %w", err)
		}
	}

	// 解码图片尺寸
	cfg, _, err := image.DecodeConfig(rs)
	if err != nil {
		return nil, fmt.Errorf("解析图片尺寸失败: %w", err)
	}

	width, height := cfg.Width, cfg.Height
	if exifData != nil {
		width, height = orientedDimensions(width, height, exifData.Orientation)
		exifData.Width = width
		exifData.Height = height
	}

	return &Meta{
		Width:    width,
		Height:   height,
		MimeType: mimeType,
		TakenAt:  takenAt,
		EXIF:     exifData,
	}, nil
}

// parseEXIF 从 JPEG 中解析 EXIF 数据
func parseEXIF(rs io.ReadSeeker) (*EXIFData, time.Time, error) {
	x, err := exif.Decode(rs)
	if err != nil {
		return nil, time.Time{}, err
	}

	data := &EXIFData{}
	var takenAt time.Time

	// 拍摄时间
	if t, err := x.DateTime(); err == nil {
		takenAt = t
		data.TakenAt = t
	}

	// 相机品牌
	if tag, err := x.Get(exif.Make); err == nil {
		data.Make, _ = tag.StringVal()
	}

	// 相机型号
	if tag, err := x.Get(exif.Model); err == nil {
		data.Model, _ = tag.StringVal()
	}

	// 方向信息，常见手机照片会依赖该字段决定显示方向
	if tag, err := x.Get(exif.Orientation); err == nil {
		if v, err := tag.Int(0); err == nil {
			data.Orientation = v
		}
	}

	// 光圈
	if tag, err := x.Get(exif.FNumber); err == nil {
		data.FNumber = tag.String()
	}

	// 曝光时间
	if tag, err := x.Get(exif.ExposureTime); err == nil {
		data.ExposureTime = tag.String()
	}

	// ISO
	if tag, err := x.Get(exif.ISOSpeedRatings); err == nil {
		if v, err := tag.Int(0); err == nil {
			data.ISOSpeed = v
		}
	}

	// 焦距
	if tag, err := x.Get(exif.FocalLength); err == nil {
		data.FocalLength = tag.String()
	}

	// GPS
	if lat, lon, err := x.LatLong(); err == nil {
		data.Latitude = lat
		data.Longitude = lon
		data.HasGPS = true
		data.Location = reverseGeocodeLocation(lat, lon)
	}

	return data, takenAt, nil
}

func reverseGeocodeLocation(lat, lon float64) string {
	if location := reverseGeocodeViaAmap(lat, lon); location != "" {
		return location
	}
	if location := reverseGeocodeViaNominatim(lat, lon); location != "" {
		return location
	}
	return ""
}

func reverseGeocodeViaAmap(lat, lon float64) string {
	key := strings.TrimSpace(os.Getenv("ECHO_GALLERY_AMAP_KEY"))
	if key == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1800*time.Millisecond)
	defer cancel()
	endpoint := "https://restapi.amap.com/v3/geocode/regeo"
	params := url.Values{}
	params.Set("location", fmt.Sprintf("%.7f,%.7f", lon, lat))
	params.Set("key", key)
	params.Set("radius", "1000")
	params.Set("extensions", "base")
	params.Set("batch", "false")
	params.Set("roadlevel", "0")
	params.Set("homeorcorp", "0")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "EchoGallery/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	var payload struct {
		Status    string `json:"status"`
		Regeocode struct {
			FormattedAddress string `json:"formatted_address"`
			AddressComponent struct {
				Province string `json:"province"`
				City     string `json:"city"`
				District string `json:"district"`
				Township string `json:"township"`
				Street   string `json:"street"`
				Number   string `json:"number"`
			} `json:"addressComponent"`
		} `json:"regeocode"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&payload); err != nil {
		return ""
	}
	if strings.TrimSpace(payload.Status) != "1" {
		return ""
	}
	if text := strings.TrimSpace(payload.Regeocode.FormattedAddress); text != "" {
		return text
	}
	ac := payload.Regeocode.AddressComponent
	return strings.TrimSpace(strings.Join([]string{
		ac.Province,
		ac.City,
		ac.District,
		ac.Township,
		ac.Street,
		ac.Number,
	}, ""))
}

func reverseGeocodeViaNominatim(lat, lon float64) string {
	ctx, cancel := context.WithTimeout(context.Background(), 1800*time.Millisecond)
	defer cancel()

	endpoint := "https://nominatim.openstreetmap.org/reverse"
	params := url.Values{}
	params.Set("format", "jsonv2")
	params.Set("lat", fmt.Sprintf("%.7f", lat))
	params.Set("lon", fmt.Sprintf("%.7f", lon))
	params.Set("accept-language", "zh-CN,zh;q=0.9,en;q=0.5")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "EchoGallery/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	var payload struct {
		DisplayName string `json:"display_name"`
		Name        string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&payload); err != nil {
		return ""
	}
	return strings.TrimSpace(firstNonEmpty(payload.Name, payload.DisplayName))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func orientedDimensions(width, height, orientation int) (int, int) {
	switch orientation {
	case 5, 6, 7, 8:
		return height, width
	default:
		return width, height
	}
}

// DetectMimeType 根据文件名后缀检测 MIME 类型
func DetectMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	// mime.TypeByExtension 在不同平台行为可能不一致，手动补充常见类型
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".tif", ".tiff":
		return "image/tiff"
	default:
		t := mime.TypeByExtension(ext)
		if t == "" {
			return "application/octet-stream"
		}
		return t
	}
}
