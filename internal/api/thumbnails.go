package api

import (
	"net/http"
	"path/filepath"
	"strings"

	"video-editor/internal/engine"
	"video-editor/internal/model"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleGenerateThumbnails(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	seqID := chi.URLParam(r, "seqID")

	project, err := s.store.GetProject(projectID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, "project not found")
			return
		}
		internalError(w, "failed to get project")
		return
	}

	seq, err := s.store.GetSequence(projectID, seqID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, "sequence not found")
			return
		}
		internalError(w, "failed to get sequence")
		return
	}

	// Build asset map
	assetMap := map[string]*model.Asset{}
	for i := range project.Assets {
		assetMap[project.Assets[i].ID] = &project.Assets[i]
	}

	thumbDir := s.store.ThumbnailsPath(projectID)
	results, err := engine.GenerateSequenceThumbnails(seq, assetMap, thumbDir)
	if err != nil {
		internalError(w, "failed to generate thumbnails")
		return
	}

	// Add full URL paths
	type thumbResponse struct {
		ClipID       string `json:"clip_id"`
		ThumbnailURL string `json:"thumbnail_url"`
	}
	var response []thumbResponse
	for _, t := range results {
		response = append(response, thumbResponse{
			ClipID:       t.ClipID,
			ThumbnailURL: "/api/v1/projects/" + projectID + "/thumbnails/" + t.Filename,
		})
	}
	if response == nil {
		response = []thumbResponse{}
	}

	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleServeThumbnail(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	filename := chi.URLParam(r, "filename")

	// Sanitize - no path traversal
	filename = filepath.Base(filename)
	if strings.Contains(filename, "..") {
		badRequest(w, "invalid filename")
		return
	}

	thumbPath := filepath.Join(s.store.ThumbnailsPath(projectID), filename)
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeFile(w, r, thumbPath)
}
