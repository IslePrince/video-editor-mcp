package model

import "time"

type AssetType string

const (
	AssetTypeVideo    AssetType = "video"
	AssetTypeAudio    AssetType = "audio"
	AssetTypeImage    AssetType = "image"
	AssetTypeSubtitle AssetType = "subtitle"
)

type ProxyStatus string

const (
	ProxyStatusNone       ProxyStatus = "none"
	ProxyStatusGenerating ProxyStatus = "generating"
	ProxyStatusReady      ProxyStatus = "ready"
	ProxyStatusFailed     ProxyStatus = "failed"
)

type AssetMetadata struct {
	Duration    float64 `json:"duration,omitempty"`
	Width       int     `json:"width,omitempty"`
	Height      int     `json:"height,omitempty"`
	Codec       string  `json:"codec,omitempty"`
	PixelFormat string  `json:"pixel_format,omitempty"`
	SampleRate  int     `json:"sample_rate,omitempty"`
	Channels    int     `json:"channels,omitempty"`
	BitRate     int64   `json:"bit_rate,omitempty"`
	FileSize    int64   `json:"file_size"`
	AudioCodec  string  `json:"audio_codec,omitempty"`
}

type Asset struct {
	ID             string        `json:"id"`
	ProjectID      string        `json:"project_id"`
	Name           string        `json:"name"`
	FilePath       string        `json:"file_path"`
	Type           AssetType     `json:"type"`
	ViewType       string        `json:"view_type,omitempty"`
	RecordingGroup string        `json:"recording_group,omitempty"`
	Metadata       AssetMetadata `json:"metadata"`
	ProxyStatus    ProxyStatus   `json:"proxy_status"`
	ProxyPath      string        `json:"proxy_path,omitempty"`
	ThumbnailPath  string        `json:"thumbnail_path,omitempty"`
	WaveformPath   string        `json:"waveform_path,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
}
