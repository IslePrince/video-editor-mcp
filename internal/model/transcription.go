package model

import "time"

type TranscriptionStatus string

const (
	TranscriptionStatusQueued      TranscriptionStatus = "queued"
	TranscriptionStatusExtracting  TranscriptionStatus = "extracting_audio"
	TranscriptionStatusTranscribing TranscriptionStatus = "transcribing"
	TranscriptionStatusComplete    TranscriptionStatus = "complete"
	TranscriptionStatusFailed      TranscriptionStatus = "failed"
)

type TranscriptionJob struct {
	ID            string              `json:"id"`
	ProjectID     string              `json:"project_id"`
	AssetID       string              `json:"asset_id"`
	Model         string              `json:"model"`
	Language      string              `json:"language,omitempty"`
	Status        TranscriptionStatus `json:"status"`
	Progress      float64             `json:"progress"`
	OutputAssetID string              `json:"output_asset_id,omitempty"`
	OutputPath    string              `json:"output_path,omitempty"`
	Error         string              `json:"error,omitempty"`
	UseGPU        bool                `json:"use_gpu"`
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
}
