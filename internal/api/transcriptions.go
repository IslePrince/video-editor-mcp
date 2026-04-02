package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"video-editor/internal/engine"
	"video-editor/internal/model"
	"video-editor/internal/queue"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type transcribeRequest struct {
	AssetID  string `json:"asset_id"`
	Model    string `json:"model,omitempty"`
	Language string `json:"language,omitempty"`
}

func (s *Server) handleTranscribe(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	var req transcribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if req.AssetID == "" {
		badRequest(w, "asset_id is required")
		return
	}

	// Verify asset exists and is video or audio
	asset, err := s.store.GetAsset(projectID, req.AssetID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, "asset not found")
			return
		}
		internalError(w, "failed to get asset")
		return
	}
	if asset.Type != model.AssetTypeVideo && asset.Type != model.AssetTypeAudio {
		badRequest(w, "asset must be a video or audio file")
		return
	}

	// Resolve model
	whisperModel := req.Model
	if whisperModel == "" {
		whisperModel = s.whisperModel
	}
	if whisperModel == "" {
		whisperModel = "base"
	}

	// Detect GPU availability
	useGPU := s.enableGPU && engine.DetectGPU()

	// Create transcription job
	job := &model.TranscriptionJob{
		ID:        uuid.New().String(),
		ProjectID: projectID,
		AssetID:   req.AssetID,
		Model:     whisperModel,
		Language:  req.Language,
		Status:    model.TranscriptionStatusQueued,
		Progress:  0,
		UseGPU:    useGPU,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.store.SaveTranscriptionJob(job); err != nil {
		internalError(w, "failed to save transcription job")
		return
	}

	// Ensure output directory
	s.store.EnsureProjectDirs(projectID)
	outputDir := s.store.TranscriptionsPath(projectID)

	// Submit to queue
	s.renderQueue.Push(queue.Job{
		ID:        job.ID,
		ProjectID: projectID,
		Execute: func() error {
			j, err := s.store.GetTranscriptionJob(projectID, job.ID)
			if err != nil {
				return err
			}

			tReq := engine.TranscribeRequest{
				Job:       j,
				InputPath: asset.FilePath,
				OutputDir: outputDir,
				SaveJob: func(j *model.TranscriptionJob) error {
					return s.store.SaveTranscriptionJob(j)
				},
			}

			if err := engine.RunTranscription(context.Background(), tReq); err != nil {
				j.Status = model.TranscriptionStatusFailed
				j.Error = err.Error()
				j.UpdatedAt = time.Now().UTC()
				s.store.SaveTranscriptionJob(j)
				return err
			}

			// Auto-import the VTT as a subtitle asset
			j, _ = s.store.GetTranscriptionJob(projectID, job.ID)
			if j.OutputPath != "" {
				subtitleAsset := &model.Asset{
					ID:        uuid.New().String(),
					ProjectID: projectID,
					Name:      asset.Name + " (transcript)",
					FilePath:  j.OutputPath,
					Type:      model.AssetTypeSubtitle,
					CreatedAt: time.Now().UTC(),
				}
				if err := s.store.CreateAsset(projectID, subtitleAsset); err != nil {
					log.Printf("[transcribe] failed to auto-import subtitle asset: %v", err)
				} else {
					j.OutputAssetID = subtitleAsset.ID
					j.UpdatedAt = time.Now().UTC()
					s.store.SaveTranscriptionJob(j)
					log.Printf("[transcribe] auto-imported subtitle asset %s for job %s", subtitleAsset.ID, j.ID)
				}
			}

			return nil
		},
	})

	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleGetTranscription(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	jobID := chi.URLParam(r, "jobID")

	job, err := s.store.GetTranscriptionJob(projectID, jobID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, "transcription job not found")
			return
		}
		internalError(w, "failed to get transcription job")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleListTranscriptions(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	jobs, err := s.store.ListTranscriptionJobs(projectID)
	if err != nil {
		internalError(w, "failed to list transcription jobs")
		return
	}
	if jobs == nil {
		jobs = []model.TranscriptionJob{}
	}
	writeJSON(w, http.StatusOK, jobs)
}
