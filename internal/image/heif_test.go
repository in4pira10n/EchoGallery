package image

import (
	"context"
	"errors"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProbeHEIFReadsImageDimensionsAndCaptureTime(t *testing.T) {
	meta, err := probeHEIF("photo.heic", func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "ffprobe" || args[len(args)-1] != "photo.heic" {
			t.Fatalf("unexpected ffprobe invocation: %s %v", name, args)
		}
		return []byte(`{"streams":[{"width":512,"height":512,"tags":{"Make":"Apple","Model":"Phone","DateTimeOriginal":"2025:04:03 12:11:10"},"side_data_list":[{"rotation":90}]}],"stream_groups":[{"type":"Tile Grid","components":[{"width":4032,"height":3024,"subcomponents":[{"stream_index":0,"tile_horizontal_offset":0,"tile_vertical_offset":0},{"stream_index":1,"tile_horizontal_offset":512,"tile_vertical_offset":0}]}]}],"format":{"tags":{}}}`), nil
	})
	if err != nil {
		t.Fatalf("probe HEIF failed: %v", err)
	}
	if meta.Width != 4032 || meta.Height != 3024 || meta.Orientation != 6 {
		t.Fatalf("unexpected dimensions or orientation: %+v", meta)
	}
	if meta.Make != "Apple" || meta.Model != "Phone" {
		t.Fatalf("unexpected camera metadata: %+v", meta)
	}
	if got := meta.TakenAt.Format("2006-01-02 15:04:05"); got != "2025-04-03 12:11:10" {
		t.Fatalf("capture time = %q", got)
	}
}

func TestProbeHEIFReportsMissingFFprobe(t *testing.T) {
	_, err := probeHEIF("photo.heif", func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, exec.ErrNotFound
	})
	if !errors.Is(err, ErrHEIFProbeUnavailable) {
		t.Fatalf("expected missing ffprobe error, got %v", err)
	}
}

func TestConvertHEIFToJPEGWritesValidatedAtomicOutput(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "photo.heif-preview.jpg")
	if err := os.WriteFile(output, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	called := false
	err := convertHEIFToJPEGWithRunner("source.heic", output, func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "ffprobe" {
			return []byte(`{"streams":[{"width":40,"height":30}]}`), nil
		}
		called = true
		if name != "ffmpeg" || args[len(args)-1] == output {
			t.Fatalf("conversion should use a temporary output path: %s %v", name, args)
		}
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "-map 0:v:0") || !strings.Contains(joined, "-frames:v 1") || !strings.Contains(joined, "-update 1") {
			t.Fatalf("expected one full-resolution still frame: %v", args)
		}
		return nil, os.WriteFile(args[len(args)-1], createTestJPEG(40, 30), 0644)
	})
	if err != nil {
		t.Fatalf("convert HEIF failed: %v", err)
	}
	if !called {
		t.Fatal("ffmpeg was not called")
	}
	file, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	config, err := jpeg.DecodeConfig(file)
	_ = file.Close()
	if err != nil || config.Width != 40 || config.Height != 30 {
		t.Fatalf("converted JPEG is invalid: config=%+v err=%v", config, err)
	}
}

func TestConvertHEIFToJPEGReportsMissingFFmpeg(t *testing.T) {
	err := convertHEIFToJPEGWithRunner("source.heic", filepath.Join(t.TempDir(), "preview.jpg"), func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if name == "ffprobe" {
			return []byte(`{"streams":[{"width":40,"height":30}]}`), nil
		}
		return nil, exec.ErrNotFound
	})
	if !errors.Is(err, ErrHEIFConverterUnavailable) {
		t.Fatalf("expected missing ffmpeg error, got %v", err)
	}
}

func TestConvertHEIFToJPEGReportsOutputRenameFailure(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "destination")
	if err := os.Mkdir(output, 0755); err != nil {
		t.Fatal(err)
	}
	err := convertHEIFToJPEGWithRunner("source.heic", output, func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "-v" {
			return []byte(`{"streams":[{"width":40,"height":30}]}`), nil
		}
		return nil, os.WriteFile(args[len(args)-1], createTestJPEG(40, 30), 0644)
	})
	if err == nil || !strings.Contains(err.Error(), "保存 HEIF 预览缓存失败") {
		t.Fatalf("expected output rename error, got %v", err)
	}
}

func TestHEIFConversionSelectionComposesTileGrid(t *testing.T) {
	data := &heifProbeOutput{}
	data.StreamGroups = append(data.StreamGroups, struct {
		Type       string                  `json:"type"`
		Components []heifTileGridComponent `json:"components"`
	}{
		Type: "Tile Grid",
		Components: []heifTileGridComponent{{
			Width: 6, Height: 4,
			Subcomponents: []heifTile{
				{StreamIndex: 0, HorizontalOffset: 0, VerticalOffset: 0},
				{StreamIndex: 1, HorizontalOffset: 4, VerticalOffset: 0},
				{StreamIndex: 2, HorizontalOffset: 0, VerticalOffset: 2},
				{StreamIndex: 3, HorizontalOffset: 4, VerticalOffset: 2},
			},
		}},
	})
	graph, mapped, err := heifConversionSelection(data)
	if err != nil {
		t.Fatalf("build tile grid conversion: %v", err)
	}
	if mapped != "[heif_preview]" {
		t.Fatalf("mapped stream = %q", mapped)
	}
	for _, expected := range []string{
		"[0:0][0:1][0:2][0:3]",
		"xstack=inputs=4:layout=0_0|4_0|0_2|4_2:shortest=1",
		"crop=6:4:0:0[heif_preview]",
	} {
		if !strings.Contains(graph, expected) {
			t.Errorf("filter graph %q does not contain %q", graph, expected)
		}
	}
}

func TestConvertHEIFToJPEGUsesGridStreams(t *testing.T) {
	output := filepath.Join(t.TempDir(), "grid.jpg")
	ffmpegCalled := false
	err := convertHEIFToJPEGWithRunner("grid.heic", output, func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "ffprobe" {
			return []byte(`{"streams":[{"width":512,"height":512}],"stream_groups":[{"type":"Tile Grid","components":[{"width":6,"height":4,"subcomponents":[{"stream_index":0,"tile_horizontal_offset":0,"tile_vertical_offset":0},{"stream_index":1,"tile_horizontal_offset":4,"tile_vertical_offset":0},{"stream_index":2,"tile_horizontal_offset":0,"tile_vertical_offset":2},{"stream_index":3,"tile_horizontal_offset":4,"tile_vertical_offset":2}]}]}]}`), nil
		}
		ffmpegCalled = true
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "xstack=inputs=4") || !strings.Contains(joined, "-map [heif_preview]") {
			t.Fatalf("expected grid reconstruction filter, got %v", args)
		}
		return nil, os.WriteFile(args[len(args)-1], createTestJPEG(6, 4), 0644)
	})
	if err != nil {
		t.Fatalf("convert tile grid HEIF: %v", err)
	}
	if !ffmpegCalled {
		t.Fatal("ffmpeg was not called")
	}
}

func TestHEIFOrientationFiltersNormalizeDerivedPreview(t *testing.T) {
	for _, test := range []struct {
		orientation int
		want        string
	}{
		{2, "hflip"},
		{3, "hflip,vflip"},
		{6, "transpose=clock"},
		{8, "transpose=cclock"},
	} {
		if got := heifOrientationFilter(test.orientation); got != test.want {
			t.Fatalf("orientation %d: got %q, want %q", test.orientation, got, test.want)
		}
	}
	if got := heifOrientationFilter(1); got != "" {
		t.Fatalf("normal orientation should not add a transform: %q", got)
	}
}
