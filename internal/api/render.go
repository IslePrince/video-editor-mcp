package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"video-editor/internal/engine"
	"video-editor/internal/model"
	"video-editor/internal/queue"
	renderPkg "video-editor/internal/render"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type submitRenderRequest struct {
	SequenceID  string              `json:"sequence_id"`
	ProfileName string              `json:"profile_name,omitempty"`
	Profile     *model.RenderProfile `json:"profile,omitempty"`
}

func (s *Server) handleSubmitRender(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	project, err := s.store.GetProject(projectID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, "project not found")
			return
		}
		internalError(w, "failed to get project")
		return
	}

	var req submitRenderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if req.SequenceID == "" {
		badRequest(w, "sequence_id is required")
		return
	}

	// Get sequence
	seq, err := s.store.GetSequence(projectID, req.SequenceID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, "sequence not found")
			return
		}
		internalError(w, "failed to get sequence")
		return
	}

	// Resolve profile
	var profile model.RenderProfile
	if req.Profile != nil {
		profile = *req.Profile
	} else if req.ProfileName != "" {
		p, ok := renderPkg.GetProfile(req.ProfileName)
		if !ok {
			badRequest(w, "unknown profile: "+req.ProfileName)
			return
		}
		profile = p
	} else {
		profile, _ = renderPkg.GetProfile("h264_medium")
	}

	// Build asset map
	assetMap := map[string]*model.Asset{}
	for i := range project.Assets {
		assetMap[project.Assets[i].ID] = &project.Assets[i]
	}

	// Resolve and remap subtitles
	var subtitlePath string
	if seq.SubtitleAssetID != "" {
		if subAsset, ok := assetMap[seq.SubtitleAssetID]; ok {
			// Parse original VTT
			origEntries, parseErr := engine.ParseVTT(subAsset.FilePath)
			log.Printf("[render] parsed VTT %s: %d entries, err=%v", subAsset.FilePath, len(origEntries), parseErr)
			if parseErr == nil && len(origEntries) > 0 {
				// Collect video clips in order
				var videoClips []model.Clip
				for _, track := range seq.Tracks {
					if track.Type == model.TrackTypeVideo && !track.Muted {
						for _, clip := range track.Clips {
							if clip.Enabled {
								videoClips = append(videoClips, clip)
							}
						}
					}
				}
				// Sort by timeline_in
				for i := 0; i < len(videoClips); i++ {
					for j := i + 1; j < len(videoClips); j++ {
						if videoClips[j].TimelineIn < videoClips[i].TimelineIn {
							videoClips[i], videoClips[j] = videoClips[j], videoClips[i]
						}
					}
				}
				// Build clip mappings and remap
				mappings := engine.BuildClipMappings(seq, videoClips, 1.0)
				remapped := engine.RemapSubtitles(origEntries, mappings)
				log.Printf("[render] remapped subtitles: %d entries from %d clips", len(remapped), len(videoClips))
				if len(remapped) > 0 {
					// Write remapped VTT to temp file
					s.store.EnsureProjectDirs(projectID)
					tempVTT := filepath.Join(s.store.ThumbnailsPath(projectID), "remapped_"+uuid.New().String()+".vtt")
					if writeErr := engine.WriteVTT(tempVTT, remapped); writeErr == nil {
						subtitlePath = tempVTT
						log.Printf("[render] wrote remapped VTT: %s", tempVTT)
					} else {
						log.Printf("[render] failed to write VTT: %v", writeErr)
					}
				}
			}
		}
	}

	// Build filter graph
	fg, err := engine.BuildFilterGraph(seq, assetMap, project.Settings, subtitlePath)
	if err != nil {
		badRequest(w, "filter graph error: "+err.Error())
		return
	}

	// Create render job
	job := &model.RenderJob{
		ID:         uuid.New().String(),
		ProjectID:  projectID,
		SequenceID: req.SequenceID,
		Profile:    profile,
		Status:     model.RenderStatusQueued,
		Progress:   0,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	if err := s.store.SaveRenderJob(job); err != nil {
		internalError(w, "failed to save render job")
		return
	}

	// Submit to queue
	executor := renderPkg.NewRenderExecutor(s.store)
	s.renderQueue.Push(queue.Job{
		ID:        job.ID,
		ProjectID: projectID,
		Execute: func() error {
			j, err := s.store.GetRenderJob(projectID, job.ID)
			if err != nil {
				return err
			}
			if err := executor.Execute(context.Background(), j, fg); err != nil {
				j.Status = model.RenderStatusFailed
				j.Error = err.Error()
				j.UpdatedAt = time.Now().UTC()
				s.store.SaveRenderJob(j)
				return err
			}
			return nil
		},
	})

	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleListRenders(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	jobs, err := s.store.ListRenderJobs(projectID)
	if err != nil {
		internalError(w, "failed to list render jobs")
		return
	}
	if jobs == nil {
		jobs = []model.RenderJob{}
	}
	writeJSON(w, http.StatusOK, jobs)
}

func (s *Server) handleGetRender(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	renderID := chi.URLParam(r, "renderID")

	job, err := s.store.GetRenderJob(projectID, renderID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, "render job not found")
			return
		}
		internalError(w, "failed to get render job")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleDownloadRender(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	renderID := chi.URLParam(r, "renderID")

	job, err := s.store.GetRenderJob(projectID, renderID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, "render job not found")
			return
		}
		internalError(w, "failed to get render job")
		return
	}

	if job.Status != model.RenderStatusComplete {
		badRequest(w, "render not complete, status: "+string(job.Status))
		return
	}

	f, err := os.Open(job.OutputPath)
	if err != nil {
		internalError(w, "output file not found")
		return
	}
	defer f.Close()

	stat, _ := f.Stat()
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", "attachment; filename=render_"+renderID+".mp4")
	http.ServeContent(w, r, "render.mp4", stat.ModTime(), f)
}

func (s *Server) handleCopyRenderToMedia(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	renderID := chi.URLParam(r, "renderID")

	job, err := s.store.GetRenderJob(projectID, renderID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, "render job not found")
			return
		}
		internalError(w, "failed to get render job")
		return
	}

	if job.Status != model.RenderStatusComplete {
		badRequest(w, "render not complete, status: "+string(job.Status))
		return
	}

	var req struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if req.Filename == "" {
		badRequest(w, "filename is required")
		return
	}

	// Sanitize filename — strip path separators
	req.Filename = filepath.Base(req.Filename)
	if !strings.HasSuffix(strings.ToLower(req.Filename), ".mp4") {
		req.Filename += ".mp4"
	}

	mediaDir := s.mediaPath
	if mediaDir == "" {
		mediaDir = "/media"
	}

	destPath := filepath.Join(mediaDir, req.Filename)

	// Copy render output to media directory
	src, err := os.Open(job.OutputPath)
	if err != nil {
		internalError(w, "render output file not found")
		return
	}
	defer src.Close()

	dst, err := os.Create(destPath)
	if err != nil {
		internalError(w, "failed to create output file in media directory: "+err.Error())
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		internalError(w, "failed to copy render to media directory")
		return
	}

	stat, _ := os.Stat(destPath)
	sizeMB := float64(stat.Size()) / 1024 / 1024

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":    destPath,
		"filename": req.Filename,
		"size_mb":  fmt.Sprintf("%.1f", sizeMB),
		"message":  "Render copied to media directory",
	})
}

func (s *Server) handleListRenderProfiles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, renderPkg.ListProfiles())
}
