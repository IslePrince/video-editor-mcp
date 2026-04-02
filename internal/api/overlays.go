package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"video-editor/internal/model"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ── Image Overlays ──────────────────────────────────────────────────────────

type addOverlayRequest struct {
	AssetID   string  `json:"asset_id"`
	Position  string  `json:"position"`
	X         int     `json:"x,omitempty"`
	Y         int     `json:"y,omitempty"`
	Width     string  `json:"width,omitempty"`
	Opacity   float64 `json:"opacity"`
	StartTime float64 `json:"start_time,omitempty"`
	EndTime   float64 `json:"end_time,omitempty"`
	Padding   int     `json:"padding,omitempty"`
}

func (s *Server) handleAddOverlay(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	seqID := chi.URLParam(r, "seqID")

	seq, err := s.store.GetSequence(projectID, seqID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, err.Error())
			return
		}
		internalError(w, "failed to get sequence")
		return
	}

	var req addOverlayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if req.AssetID == "" {
		badRequest(w, "asset_id is required")
		return
	}
	if req.Position == "" {
		req.Position = "top_right"
	}
	if req.Opacity == 0 {
		req.Opacity = 1.0
	}
	if req.Padding == 0 {
		req.Padding = 20
	}

	// Validate asset exists and is an image
	asset, err := s.store.GetAsset(projectID, req.AssetID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, "asset not found")
			return
		}
		internalError(w, "failed to get asset")
		return
	}
	if asset.Type != model.AssetTypeImage {
		badRequest(w, "asset must be of type 'image' for overlay")
		return
	}

	overlay := model.Overlay{
		ID:        uuid.New().String(),
		AssetID:   req.AssetID,
		Position:  req.Position,
		X:         req.X,
		Y:         req.Y,
		Width:     req.Width,
		Opacity:   req.Opacity,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		Padding:   req.Padding,
	}

	seq.Overlays = append(seq.Overlays, overlay)
	if err := s.store.UpdateSequence(projectID, seq); err != nil {
		internalError(w, "failed to add overlay")
		return
	}
	writeJSON(w, http.StatusCreated, overlay)
}

func (s *Server) handleDeleteOverlay(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	seqID := chi.URLParam(r, "seqID")
	overlayID := chi.URLParam(r, "overlayID")

	seq, err := s.store.GetSequence(projectID, seqID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, err.Error())
			return
		}
		internalError(w, "failed to get sequence")
		return
	}

	found := false
	overlays := make([]model.Overlay, 0, len(seq.Overlays))
	for _, o := range seq.Overlays {
		if o.ID == overlayID {
			found = true
			continue
		}
		overlays = append(overlays, o)
	}
	if !found {
		notFound(w, "overlay not found")
		return
	}

	seq.Overlays = overlays
	if err := s.store.UpdateSequence(projectID, seq); err != nil {
		internalError(w, "failed to delete overlay")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Text Overlays ───────────────────────────────────────────────────────────

type addTextOverlayRequest struct {
	Text       string  `json:"text"`
	Position   string  `json:"position,omitempty"`
	X          int     `json:"x,omitempty"`
	Y          int     `json:"y,omitempty"`
	FontFamily string  `json:"font_family,omitempty"`
	FontSize   int     `json:"font_size,omitempty"`
	FontColor  string  `json:"font_color,omitempty"`
	Bold       bool    `json:"bold,omitempty"`
	BgColor    string  `json:"bg_color,omitempty"`
	BgOpacity  float64 `json:"bg_opacity,omitempty"`
	Padding    int     `json:"padding,omitempty"`
	StartTime  float64 `json:"start_time"`
	EndTime    float64 `json:"end_time"`
	Animation  string  `json:"animation,omitempty"`
}

func (s *Server) handleAddTextOverlay(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	seqID := chi.URLParam(r, "seqID")

	seq, err := s.store.GetSequence(projectID, seqID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, err.Error())
			return
		}
		internalError(w, "failed to get sequence")
		return
	}

	var req addTextOverlayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if req.Text == "" {
		badRequest(w, "text is required")
		return
	}
	if req.Position == "" {
		req.Position = "bottom"
	}
	if req.FontFamily == "" {
		req.FontFamily = "DM Sans"
	}
	if req.FontSize == 0 {
		req.FontSize = 36
	}
	if req.FontColor == "" {
		req.FontColor = "#FFFFFF"
	}
	if req.Padding == 0 {
		req.Padding = 10
	}
	if req.Animation == "" {
		req.Animation = "none"
	}

	overlay := model.TextOverlay{
		ID:         uuid.New().String(),
		Text:       req.Text,
		Position:   req.Position,
		X:          req.X,
		Y:          req.Y,
		FontFamily: req.FontFamily,
		FontSize:   req.FontSize,
		FontColor:  req.FontColor,
		Bold:       req.Bold,
		BgColor:    req.BgColor,
		BgOpacity:  req.BgOpacity,
		Padding:    req.Padding,
		StartTime:  req.StartTime,
		EndTime:    req.EndTime,
		Animation:  req.Animation,
	}

	seq.TextOverlays = append(seq.TextOverlays, overlay)
	if err := s.store.UpdateSequence(projectID, seq); err != nil {
		internalError(w, "failed to add text overlay")
		return
	}
	writeJSON(w, http.StatusCreated, overlay)
}

func (s *Server) handleDeleteTextOverlay(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	seqID := chi.URLParam(r, "seqID")
	overlayID := chi.URLParam(r, "overlayID")

	seq, err := s.store.GetSequence(projectID, seqID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, err.Error())
			return
		}
		internalError(w, "failed to get sequence")
		return
	}

	found := false
	overlays := make([]model.TextOverlay, 0, len(seq.TextOverlays))
	for _, o := range seq.TextOverlays {
		if o.ID == overlayID {
			found = true
			continue
		}
		overlays = append(overlays, o)
	}
	if !found {
		notFound(w, "text overlay not found")
		return
	}

	seq.TextOverlays = overlays
	if err := s.store.UpdateSequence(projectID, seq); err != nil {
		internalError(w, "failed to delete text overlay")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Subtitle Style ──────────────────────────────────────────────────────────

func (s *Server) handleSetSubtitleStyle(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	seqID := chi.URLParam(r, "seqID")

	seq, err := s.store.GetSequence(projectID, seqID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, err.Error())
			return
		}
		internalError(w, "failed to get sequence")
		return
	}

	var style model.SubtitleStyle
	if err := json.NewDecoder(r.Body).Decode(&style); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	seq.SubtitleStyle = &style
	if err := s.store.UpdateSequence(projectID, seq); err != nil {
		internalError(w, "failed to set subtitle style")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"subtitle_style": style,
		"message":        "Subtitle style updated",
	})
}
