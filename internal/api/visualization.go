package api

import (
	"image/png"
	"net/http"
	"strings"

	"video-editor/internal/engine"
	"video-editor/internal/model"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleTimelineImage(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	seqID := chi.URLParam(r, "seqID")

	project, err := s.store.GetProject(projectID)
	if err != nil {
		http.Error(w, "project not found", 404)
		return
	}

	// If no seqID, use first sequence
	var seq *model.Sequence
	if seqID != "" {
		seq, err = s.store.GetSequence(projectID, seqID)
		if err != nil {
			http.Error(w, "sequence not found", 404)
			return
		}
	} else if len(project.Sequences) > 0 {
		seq, err = s.store.GetSequence(projectID, project.Sequences[0].ID)
		if err != nil {
			seq = &project.Sequences[0]
		}
	}

	if seq == nil {
		// Empty state
		seq = &model.Sequence{Name: "Empty"}
	}

	assetMap := buildAssetMap(project)
	thumbDir := s.store.ThumbnailsPath(projectID)

	img, err := engine.RenderTimelineImage(seq, assetMap, thumbDir)
	if err != nil {
		http.Error(w, "render failed: "+err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache")
	png.Encode(w, img)
}

func (s *Server) handleStoryboardImage(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	seqID := chi.URLParam(r, "seqID")

	project, err := s.store.GetProject(projectID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "project not found", 404)
			return
		}
		http.Error(w, "error", 500)
		return
	}

	seq, err := s.store.GetSequence(projectID, seqID)
	if err != nil {
		http.Error(w, "sequence not found", 404)
		return
	}

	assetMap := buildAssetMap(project)
	thumbDir := s.store.ThumbnailsPath(projectID)

	img, err := engine.RenderStoryboardImage(seq, assetMap, thumbDir)
	if err != nil {
		http.Error(w, "render failed: "+err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache")
	png.Encode(w, img)
}

func buildAssetMap(project *model.Project) map[string]*model.Asset {
	m := map[string]*model.Asset{}
	for i := range project.Assets {
		m[project.Assets[i].ID] = &project.Assets[i]
	}
	return m
}
