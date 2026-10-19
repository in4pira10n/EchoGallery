package danmaku

import "testing"

func TestParseBVID(t *testing.T) {
	tests := []struct {
		input string
		bvid  string
		page  int
		valid bool
	}{
		{"BV1SopR6wE2x", "BV1SopR6wE2x", 0, true},
		{"https://www.bilibili.com/video/BV1SopR6wE2x/?p=2", "BV1SopR6wE2x", 2, true},
		{"", "", 0, false},
		{"https://example.com/video/invalid", "", 0, false},
	}
	for _, test := range tests {
		bvid, page, err := ParseBVID(test.input)
		if (err == nil) != test.valid || bvid != test.bvid || page != test.page {
			t.Errorf("ParseBVID(%q) = %q, %d, %v", test.input, bvid, page, err)
		}
	}
}
