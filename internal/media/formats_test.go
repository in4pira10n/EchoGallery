package media

import "testing"

func TestDetectVideoMimeType(t *testing.T) {
	cases := []struct {
		filename string
		expected string
	}{
		{"demo.mp4", "video/mp4"},
		{"clip.MKV", "video/x-matroska"},
		{"capture.avi", "video/x-msvideo"},
		{"stream.ts", "video/mp2t"},
		{"movie.mpeg", "video/mpeg"},
		{"phone.3gp", "video/3gpp"},
		{"sample.ogv", "video/ogg"},
		{"note.txt", ""},
	}
	for _, tc := range cases {
		if got := DetectVideoMimeType(tc.filename); got != tc.expected {
			t.Fatalf("DetectVideoMimeType(%q) = %q, 期望 %q", tc.filename, got, tc.expected)
		}
	}
}
