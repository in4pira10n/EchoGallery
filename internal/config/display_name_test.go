package config

import "testing"

func TestDisplayName(t *testing.T) {
	for _, tc := range []struct{ name, want string }{{"", "EchoGallery"}, {"  家庭照片  ", "家庭照片"}} {
		cfg := &Config{Workshop: Workshop{AppName: tc.name}}
		if got := cfg.DisplayName(); got != tc.want {
			t.Fatalf("got %q, want %q", got, tc.want)
		}
	}
}
