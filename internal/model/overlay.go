package model

// Overlay represents an image overlay (e.g., logo watermark) composited on top of the video.
type Overlay struct {
	ID        string  `json:"id"`
	AssetID   string  `json:"asset_id"`
	Position  string  `json:"position"`            // top_left, top_right, bottom_left, bottom_right, center, custom
	X         int     `json:"x,omitempty"`          // custom x coordinate (when position="custom")
	Y         int     `json:"y,omitempty"`          // custom y coordinate
	Width     string  `json:"width,omitempty"`      // pixels ("150") or percentage ("15%")
	Opacity   float64 `json:"opacity"`              // 0.0-1.0, default 1.0
	StartTime float64 `json:"start_time,omitempty"` // seconds, 0 = start of sequence
	EndTime   float64 `json:"end_time,omitempty"`   // seconds, 0 = end of sequence
	Padding   int     `json:"padding,omitempty"`    // pixels from edge, default 20
}

// TextOverlay represents text burned on top of video (lower-thirds, captions, CTAs).
type TextOverlay struct {
	ID         string  `json:"id"`
	Text       string  `json:"text"`
	Position   string  `json:"position"`              // top, center, bottom, custom
	X          int     `json:"x,omitempty"`            // custom x
	Y          int     `json:"y,omitempty"`            // custom y
	FontFamily string  `json:"font_family,omitempty"`  // default "DM Sans"
	FontSize   int     `json:"font_size,omitempty"`    // default 36
	FontColor  string  `json:"font_color,omitempty"`   // hex color, default "#FFFFFF"
	Bold       bool    `json:"bold,omitempty"`
	BgColor    string  `json:"bg_color,omitempty"`     // optional box background
	BgOpacity  float64 `json:"bg_opacity,omitempty"`   // 0.0-1.0
	Padding    int     `json:"padding,omitempty"`      // box padding, default 10
	StartTime  float64 `json:"start_time"`             // seconds
	EndTime    float64 `json:"end_time"`               // seconds
	Animation  string  `json:"animation,omitempty"`    // fade_in, slide_up, none
}

// CropSettings defines how a clip should be cropped before scaling.
type CropSettings struct {
	Mode       string `json:"mode,omitempty"`        // center_crop, speaker_focus_left, speaker_focus_right
	CropX      int    `json:"crop_x,omitempty"`      // manual: top-left x
	CropY      int    `json:"crop_y,omitempty"`      // manual: top-left y
	CropWidth  int    `json:"crop_width,omitempty"`  // manual: crop region width
	CropHeight int    `json:"crop_height,omitempty"` // manual: crop region height
}

// SubtitleStyle controls the appearance of burned-in subtitles.
type SubtitleStyle struct {
	FontFamily   string  `json:"font_family,omitempty"`   // default "DM Sans Bold"
	FontSize     int     `json:"font_size,omitempty"`     // default 24
	FontColor    string  `json:"font_color,omitempty"`    // hex, default "#FFFFFF"
	OutlineColor string  `json:"outline_color,omitempty"` // hex, default "#000000"
	OutlineWidth int     `json:"outline_width,omitempty"` // pixels, default 2
	BgColor      string  `json:"bg_color,omitempty"`      // optional box background
	BgOpacity    float64 `json:"bg_opacity,omitempty"`    // 0.0-1.0
	Position     string  `json:"position,omitempty"`      // top, center, bottom
	MarginBottom int     `json:"margin_bottom,omitempty"` // pixels from bottom, default 50
}
