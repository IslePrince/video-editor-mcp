package storage

import "video-editor/internal/model"

type Storage interface {
	// Projects
	CreateProject(p *model.Project) error
	GetProject(id string) (*model.Project, error)
	ListProjects() ([]model.Project, error)
	UpdateProject(p *model.Project) error
	DeleteProject(id string) error

	// Assets
	CreateAsset(projectID string, a *model.Asset) error
	GetAsset(projectID, assetID string) (*model.Asset, error)
	ListAssets(projectID string, assetType *model.AssetType) ([]model.Asset, error)
	DeleteAsset(projectID, assetID string) error
	UpdateAsset(projectID string, a *model.Asset) error

	// Sequences
	CreateSequence(projectID string, seq *model.Sequence) error
	GetSequence(projectID, seqID string) (*model.Sequence, error)
	ListSequences(projectID string) ([]model.Sequence, error)
	UpdateSequence(projectID string, seq *model.Sequence) error
	DeleteSequence(projectID, seqID string) error

	// Render Jobs
	SaveRenderJob(job *model.RenderJob) error
	GetRenderJob(projectID, jobID string) (*model.RenderJob, error)
	ListRenderJobs(projectID string) ([]model.RenderJob, error)

	// Transcription Jobs
	SaveTranscriptionJob(job *model.TranscriptionJob) error
	GetTranscriptionJob(projectID, jobID string) (*model.TranscriptionJob, error)
	ListTranscriptionJobs(projectID string) ([]model.TranscriptionJob, error)

	// Paths
	AssetOriginalPath(projectID, assetID, ext string) string
	RenderOutputPath(projectID, jobID string) string
	TranscriptionsPath(projectID string) string
	ThumbnailsPath(projectID string) string
	EnsureProjectDirs(projectID string) error
}
