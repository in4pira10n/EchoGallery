package media

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"
)

func TestProbeVideoWithRunner_Success(t *testing.T) {
	runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte(`{
			"streams": [{"codec_type":"video","codec_name":"h264","width":1920,"height":1080,"avg_frame_rate":"30000/1001"}],
			"format": {"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"12.345"}
		}`), nil
	}

	meta, err := probeVideoWithRunner("demo.mp4", runner)
	if err != nil {
		t.Fatalf("期望成功，得到错误: %v", err)
	}
	if meta.Width != 1920 || meta.Height != 1080 {
		t.Fatalf("分辨率不正确: %+v", meta)
	}
	if meta.DurationMS != 12345 {
		t.Fatalf("期望时长 12345，得到 %d", meta.DurationMS)
	}
	if meta.CodecName != "h264" {
		t.Fatalf("期望编码 h264，得到 %s", meta.CodecName)
	}
	if meta.FrameRate < 29.96 || meta.FrameRate > 29.98 {
		t.Fatalf("期望帧率约 29.97，得到 %.4f", meta.FrameRate)
	}
}

func TestProbeVideoWithRunner_AppliesDisplayMatrixRotation(t *testing.T) {
	runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte(`{
			"streams": [{
				"codec_type":"video",
				"codec_name":"h264",
				"width":1080,
				"height":1920,
				"side_data_list":[{"side_data_type":"Display Matrix","rotation":90}]
			}],
			"format": {"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"592.126"}
		}`), nil
	}

	meta, err := probeVideoWithRunner("rotated.mp4", runner)
	if err != nil {
		t.Fatalf("期望成功，得到错误: %v", err)
	}
	if meta.Width != 1920 || meta.Height != 1080 {
		t.Fatalf("期望按 Display Matrix 修正为 1920x1080，得到 %+v", meta)
	}
}

func TestProbeVideoWithRunner_PrefersQuickTimeCreationDate(t *testing.T) {
	runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte(`{
			"streams": [{
				"codec_type":"video",
				"codec_name":"h264",
				"width":3840,
				"height":2160,
				"tags":{"creation_time":"2026-06-11T15:53:26.000000Z"}
			}],
			"format": {
				"format_name":"mov,mp4,m4a,3gp,3g2,mj2",
				"duration":"17.093333",
				"tags":{
					"creation_time":"2026-06-11T15:53:26.000000Z",
					"com.apple.quicktime.creationdate":"2026-06-07T21:03:07+0800"
				}
			}
		}`), nil
	}

	meta, err := probeVideoWithRunner("IMG_1589.MOV", runner)
	if err != nil {
		t.Fatalf("期望成功，得到错误: %v", err)
	}
	expected, err := time.Parse("2006-01-02T15:04:05-0700", "2026-06-07T21:03:07+0800")
	if err != nil {
		t.Fatalf("解析期望时间失败: %v", err)
	}
	if !meta.TakenAt.Equal(expected) {
		t.Fatalf("期望优先使用 QuickTime 拍摄时间 %v，得到 %v", expected, meta.TakenAt)
	}
}

func TestProbeVideoWithRunner_PreservesSamsungCreationTimeWallClock(t *testing.T) {
	runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte(`{
			"streams": [{
				"codec_type":"video",
				"codec_name":"h264",
				"width":1920,
				"height":1080,
				"tags":{"creation_time":"2013-02-10T11:04:24.000000Z"}
			}],
			"format": {
				"format_name":"mov,mp4,m4a,3gp,3g2,mj2",
				"duration":"8.400000",
				"tags":{
					"major_brand":"MSNV",
					"compatible_brands":"MSNVmp42isom",
					"creation_time":"2013-02-10T11:04:24.000000Z"
				}
			}
		}`), nil
	}

	meta, err := probeVideoWithRunner("SAM_0209.MP4", runner)
	if err != nil {
		t.Fatalf("期望成功，得到错误: %v", err)
	}
	expected := time.Date(2013, 2, 10, 11, 4, 24, 0, time.Local)
	if !meta.TakenAt.Equal(expected) {
		t.Fatalf("期望保留三星 MP4 墙钟时间 %v，得到 %v", expected, meta.TakenAt)
	}
}

func TestProbeVideoWithRunner_FFprobeUnavailable(t *testing.T) {
	runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return nil, exec.ErrNotFound
	}

	_, err := probeVideoWithRunner("demo.mp4", runner)
	if !errors.Is(err, ErrProbeUnavailable) {
		t.Fatalf("期望 ErrProbeUnavailable，得到 %v", err)
	}
}

func TestProbeVideoWithRunner_InvalidDuration(t *testing.T) {
	runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte(`{
			"streams": [{"codec_type":"video","codec_name":"h264","width":1280,"height":720}],
			"format": {"format_name":"mp4","duration":"oops"}
		}`), nil
	}

	_, err := probeVideoWithRunner("demo.mp4", runner)
	if !errors.Is(err, ErrInvalidVideo) {
		t.Fatalf("期望 ErrInvalidVideo，得到 %v", err)
	}
}

func TestProbeVideoWithRunner_MissingVideoMeta(t *testing.T) {
	runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte(`{
			"streams": [{"codec_type":"audio","codec_name":"aac"}],
			"format": {"format_name":"mp4","duration":"3.2"}
		}`), nil
	}

	_, err := probeVideoWithRunner("demo.mp4", runner)
	if !errors.Is(err, ErrInvalidVideo) {
		t.Fatalf("期望 ErrInvalidVideo，得到 %v", err)
	}
}
