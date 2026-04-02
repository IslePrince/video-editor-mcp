package model

type TrackType string

const (
	TrackTypeVideo TrackType = "video"
	TrackTypeAudio TrackType = "audio"
)

type Slide struct {
	Duration    float64 `json:"duration"`
	Text        string  `json:"text"`
	FontSize    int     `json:"font_size,omitempty"`
	FontColor   string  `json:"font_color,omitempty"`
	BgColor     string  `json:"bg_color,omitempty"`
	FontFamily  string  `json:"font_family,omitempty"`
	LogoAssetID string  `json:"logo_asset_id,omitempty"`
}

type Sequence struct {
	ID              string         `json:"id"`
	ProjectID       string         `json:"project_id"`
	Name            string         `json:"name"`
	Tracks          []Track        `json:"tracks,omitempty"`
	IntroSlide      *Slide         `json:"intro_slide,omitempty"`
	OutroSlide      *Slide         `json:"outro_slide,omitempty"`
	SubtitleAssetID string         `json:"subtitle_asset_id,omitempty"`
	Overlays        []Overlay      `json:"overlays,omitempty"`
	TextOverlays    []TextOverlay  `json:"text_overlays,omitempty"`
	SubtitleStyle   *SubtitleStyle `json:"subtitle_style,omitempty"`
	CropMode        string         `json:"crop_mode,omitempty"` // "fit" (default), "center_crop"
}

type Track struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Type    TrackType `json:"type"`
	Index   int       `json:"index"`
	Clips   []Clip    `json:"clips,omitempty"`
	Muted   bool      `json:"muted"`
	Locked  bool      `json:"locked"`
	Opacity float64   `json:"opacity"`
	Volume  float64   `json:"volume"`
}

type Clip struct {
	ID            string        `json:"id"`
	TrackID       string        `json:"track_id"`
	AssetID       string        `json:"asset_id"`
	TimelineIn    float64       `json:"timeline_in"`
	TimelineOut   float64       `json:"timeline_out"`
	SourceIn      float64       `json:"source_in"`
	SourceOut     float64       `json:"source_out"`
	Speed         float64       `json:"speed"`
	Effects       []Effect      `json:"effects,omitempty"`
	TransitionIn  *Transition   `json:"transition_in,omitempty"`
	TransitionOut *Transition   `json:"transition_out,omitempty"`
	Crop          *CropSettings `json:"crop,omitempty"`
	Enabled       bool          `json:"enabled"`
}
