package model

import "time"

type RenderStatus string

const (
	RenderStatusQueued     RenderStatus = "queued"
	RenderStatusPreparing  RenderStatus = "preparing"
	RenderStatusRendering  RenderStatus = "rendering"
	RenderStatusFinalizing RenderStatus = "finalizing"
	RenderStatusComplete   RenderStatus = "complete"
	RenderStatusFailed     RenderStatus = "failed"
	RenderStatusCancelled  RenderStatus = "cancelled"
)

type RenderJob struct {
	ID          string       `json:"id"`
	ProjectID   string       `json:"project_id"`
	SequenceID  string       `json:"sequence_id"`
	Profile     RenderProfile `json:"profile"`
	Status      RenderStatus `json:"status"`
	Progress    float64      `json:"progress"`
	OutputPath  string       `json:"output_path,omitempty"`
	FilterGraph string       `json:"filter_graph,omitempty"`
	Command     string       `json:"command,omitempty"`
	Error       string       `json:"error,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type RenderProfile struct {
	Name       string   `json:"name"`
	Format     string   `json:"format"`
	VideoCodec string   `json:"video_codec"`
	AudioCodec string   `json:"audio_codec"`
	Width      int      `json:"width,omitempty"`
	Height     int      `json:"height,omitempty"`
	FrameRate  Rational `json:"frame_rate,omitempty"`
	BitRate    string   `json:"bit_rate,omitempty"`
	Quality    int      `json:"quality,omitempty"`
	Passes     int      `json:"passes"`
}
