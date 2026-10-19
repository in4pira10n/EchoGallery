package config

import "encoding/json"

type DanmakuPreferences struct {
	FontStep       int    `json:"font_step"`
	FontWeightStep int    `json:"font_weight_step"`
	OpacityStep    int    `json:"opacity_step"`
	SpeedStep      int    `json:"speed_step"`
	BlockList      string `json:"block_list"`
}

func (p *DanmakuPreferences) UnmarshalJSON(data []byte) error {
	type preferencesAlias DanmakuPreferences
	var decoded preferencesAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if _, exists := fields["font_weight_step"]; !exists {
		decoded.FontWeightStep = 2
	}
	*p = DanmakuPreferences(decoded)
	return nil
}

func NormalizeDanmaku(p *DanmakuPreferences) *DanmakuPreferences {
	if p == nil {
		return &DanmakuPreferences{FontStep: 1, FontWeightStep: 2, OpacityStep: 2, SpeedStep: 2}
	}
	n := *p
	if n.FontStep < 0 || n.FontStep > 5 {
		n.FontStep = 1
	}
	if n.FontWeightStep < 0 || n.FontWeightStep > 4 {
		n.FontWeightStep = 2
	}
	if n.OpacityStep < 0 || n.OpacityStep > 4 {
		n.OpacityStep = 2
	}
	if n.SpeedStep < 0 || n.SpeedStep > 4 {
		n.SpeedStep = 2
	}
	if len(n.BlockList) > 8192 {
		n.BlockList = ""
	}
	return &n
}
