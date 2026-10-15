package storage

import "testing"

func TestNormalizeSmartAlbumDefinition(t *testing.T) {
	mode, rules, err := NormalizeSmartAlbumDefinition(" ANY ", []SmartAlbumRule{
		{Field: "filename", Operator: "contains", Value: "  trip "},
		{Field: "duration", Operator: "between", Min: "8:00", Max: "10:00"},
	})
	if err != nil {
		t.Fatalf("normalize valid definition: %v", err)
	}
	if mode != "any" || rules[0].Value != "trip" || len(rules) != 2 {
		t.Fatalf("unexpected normalized definition: mode=%q rules=%+v", mode, rules)
	}

	if _, _, err := NormalizeSmartAlbumDefinition("all", []SmartAlbumRule{{Field: "duration", Operator: "between", Min: "10:00", Max: "8:00"}}); err == nil {
		t.Fatal("expected reversed duration range to be rejected")
	}
	if _, _, err := NormalizeSmartAlbumDefinition("all", []SmartAlbumRule{{Field: "filename", Operator: "contains"}}); err == nil {
		t.Fatal("expected empty filename query to be rejected")
	}
}

func TestDescribeSmartAlbumRules(t *testing.T) {
	got := DescribeSmartAlbumRules([]SmartAlbumRule{
		{Field: "media_type", Operator: "is", Value: MediaKindVideo},
		{Field: "duration", Operator: "at_least", Min: "1:00"},
		{Field: "filename", Operator: "contains", Value: "001"},
	})
	want := "媒体类型是视频 · 视频时长大于 1 分钟 · 文件名包含 001"
	if got != want {
		t.Fatalf("unexpected smart album description: got %q, want %q", got, want)
	}
}

func TestParseSmartAlbumDuration(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{input: "8", want: 8 * 60 * 1000},
		{input: "8:05", want: (8*60 + 5) * 1000},
		{input: "1:02:03", want: (3600 + 2*60 + 3) * 1000},
	}
	for _, test := range tests {
		got, err := ParseSmartAlbumDuration(test.input)
		if err != nil || got != test.want {
			t.Errorf("ParseSmartAlbumDuration(%q) = %d, %v; want %d", test.input, got, err, test.want)
		}
	}
	for _, input := range []string{"", "1:60", "1:02:60", "-1:00", "1:2:3:4"} {
		if _, err := ParseSmartAlbumDuration(input); err == nil {
			t.Errorf("ParseSmartAlbumDuration(%q) should fail", input)
		}
	}
}
