package config

// EchoFramePreferences belong to the user's profile, never to media metadata.
type EchoFramePreferences struct {
	Source         string  `json:"source"`
	AlbumIDs       []int64 `json:"album_ids"`
	AlbumLibraryID string  `json:"album_library_id"`
	Order          string  `json:"order"`
	Interval       int     `json:"interval"`
	IntervalCustom bool    `json:"interval_custom"`
	End            string  `json:"end"`
	Transition     string  `json:"transition"`
	TextOpacity    int     `json:"text_opacity"`
	Videos         bool    `json:"videos"`
	Muted          bool    `json:"muted"`
	Clock          bool    `json:"clock"`
	Date           bool    `json:"date"`
	WakeLock       bool    `json:"wake_lock"`
}

func NormalizeEchoFrame(p *EchoFramePreferences) *EchoFramePreferences {
	if p == nil {
		p = &EchoFramePreferences{Source: "library", Order: "random", Interval: 30000, End: "reshuffle", Transition: "crossfade", TextOpacity: 60, Videos: true, Muted: true, Clock: true, Date: true, WakeLock: true}
	} else {
		copy := *p
		p = &copy
	}
	switch p.Source {
	case "library", "current", "favorites", "albums", "random":
	default:
		p.Source = "library"
	}
	switch p.Order {
	case "random", "asc", "desc":
	default:
		p.Order = "random"
	}
	if p.Interval < 3000 || p.Interval > 600000 || p.Interval%1000 != 0 {
		p.Interval = 30000
	}
	if p.TextOpacity < 10 || p.TextOpacity > 100 {
		p.TextOpacity = 60
	}
	if !p.IntervalCustom && p.Interval != 15000 && p.Interval != 30000 && p.Interval != 60000 && p.Interval != 300000 {
		p.IntervalCustom = true
	}
	switch p.End {
	case "loop", "reshuffle", "stop":
	default:
		p.End = "reshuffle"
	}
	switch p.Transition {
	case "fade", "crossfade", "zoom":
	default:
		p.Transition = "crossfade"
	}
	ids := make([]int64, 0, len(p.AlbumIDs))
	seen := make(map[int64]bool)
	for _, id := range p.AlbumIDs {
		if id > 0 && !seen[id] && len(ids) < 32 {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	p.AlbumIDs = ids
	return p
}
