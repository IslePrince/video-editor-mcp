package model

import "time"

type Rational struct {
	Num int `json:"num"`
	Den int `json:"den"`
}

type ProjectSettings struct {
	Width       int      `json:"width"`
	Height      int      `json:"height"`
	FrameRate   Rational `json:"frame_rate"`
	SampleRate  int      `json:"sample_rate"`
	PixelFormat string   `json:"pixel_format"`
	ColorSpace  string   `json:"color_space"`
}

type Project struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Settings  ProjectSettings `json:"settings"`
	Assets    []Asset         `json:"assets,omitempty"`
	Sequences []Sequence      `json:"sequences,omitempty"`
	Version   int             `json:"version"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

func DefaultProjectSettings() ProjectSettings {
	return ProjectSettings{
		Width:       1920,
		Height:      1080,
		FrameRate:   Rational{Num: 30, Den: 1},
		SampleRate:  48000,
		PixelFormat: "yuv420p",
		ColorSpace:  "bt709",
	}
}
