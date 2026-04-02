package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"video-editor/internal/model"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Sequences

type createSequenceRequest struct {
	Name            string       `json:"name"`
	IntroSlide      *model.Slide `json:"intro_slide,omitempty"`
	OutroSlide      *model.Slide `json:"outro_slide,omitempty"`
	SubtitleAssetID string       `json:"subtitle_asset_id,omitempty"`
	CropMode        string       `json:"crop_mode,omitempty"`
}

func (s *Server) handleCreateSequence(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if _, err := s.store.GetProject(projectID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, "project not found")
			return
		}
		internalError(w, "failed to get project")
		return
	}

	var req createSequenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if req.Name == "" {
		badRequest(w, "name is required")
		return
	}

	seq := &model.Sequence{
		ID:              uuid.New().String(),
		ProjectID:       projectID,
		Name:            req.Name,
		IntroSlide:      req.IntroSlide,
		OutroSlide:      req.OutroSlide,
		SubtitleAssetID: req.SubtitleAssetID,
		CropMode:        req.CropMode,
	}

	if err := s.store.CreateSequence(projectID, seq); err != nil {
		internalError(w, "failed to create sequence")
		return
	}
	writeJSONWithViz(w, http.StatusCreated, seq, projectID, seq.ID)
}

type batchSequenceRequest struct {
	Sequences []struct {
		Name            string       `json:"name"`
		Clips           []struct {
			AssetID   string  `json:"asset_id"`
			SourceIn  float64 `json:"source_in"`
			SourceOut float64 `json:"source_out"`
		} `json:"clips"`
		IntroSlide      *model.Slide `json:"intro_slide,omitempty"`
		OutroSlide      *model.Slide `json:"outro_slide,omitempty"`
		SubtitleAssetID string       `json:"subtitle_asset_id,omitempty"`
		CropMode        string       `json:"crop_mode,omitempty"`
	} `json:"sequences"`
	SharedSettings struct {
		BgColor         string `json:"bg_color,omitempty"`
		LogoAssetID     string `json:"logo_asset_id,omitempty"`
		SubtitleAssetID string `json:"subtitle_asset_id,omitempty"`
		CropMode        string `json:"crop_mode,omitempty"`
	} `json:"shared_settings"`
}

func (s *Server) handleCreateSequencesBatch(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if _, err := s.store.GetProject(projectID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, "project not found")
			return
		}
		internalError(w, "failed to get project")
		return
	}

	var req batchSequenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	var results []interface{}
	for _, seqReq := range req.Sequences {
		// Apply shared settings
		cropMode := seqReq.CropMode
		if cropMode == "" {
			cropMode = req.SharedSettings.CropMode
		}
		subtitleAssetID := seqReq.SubtitleAssetID
		if subtitleAssetID == "" {
			subtitleAssetID = req.SharedSettings.SubtitleAssetID
		}

		seq := &model.Sequence{
			ID:              uuid.New().String(),
			ProjectID:       projectID,
			Name:            seqReq.Name,
			IntroSlide:      seqReq.IntroSlide,
			OutroSlide:      seqReq.OutroSlide,
			SubtitleAssetID: subtitleAssetID,
			CropMode:        cropMode,
		}

		// Apply shared logo/bg to slides
		if seq.IntroSlide != nil && seq.IntroSlide.BgColor == "" && req.SharedSettings.BgColor != "" {
			seq.IntroSlide.BgColor = req.SharedSettings.BgColor
		}
		if seq.IntroSlide != nil && seq.IntroSlide.LogoAssetID == "" && req.SharedSettings.LogoAssetID != "" {
			seq.IntroSlide.LogoAssetID = req.SharedSettings.LogoAssetID
		}
		if seq.OutroSlide != nil && seq.OutroSlide.BgColor == "" && req.SharedSettings.BgColor != "" {
			seq.OutroSlide.BgColor = req.SharedSettings.BgColor
		}
		if seq.OutroSlide != nil && seq.OutroSlide.LogoAssetID == "" && req.SharedSettings.LogoAssetID != "" {
			seq.OutroSlide.LogoAssetID = req.SharedSettings.LogoAssetID
		}

		if err := s.store.CreateSequence(projectID, seq); err != nil {
			internalError(w, "failed to create sequence: "+seqReq.Name)
			return
		}

		// Add video track and clips
		if len(seqReq.Clips) > 0 {
			track := model.Track{
				ID:      uuid.New().String(),
				Name:    "V1",
				Type:    model.TrackTypeVideo,
				Index:   0,
				Opacity: 1.0,
				Volume:  1.0,
			}

			timelinePos := 0.0
			for _, c := range seqReq.Clips {
				dur := c.SourceOut - c.SourceIn
				clip := model.Clip{
					ID:          uuid.New().String(),
					TrackID:     track.ID,
					AssetID:     c.AssetID,
					TimelineIn:  timelinePos,
					TimelineOut: timelinePos + dur,
					SourceIn:    c.SourceIn,
					SourceOut:   c.SourceOut,
					Speed:       1.0,
					Enabled:     true,
				}
				track.Clips = append(track.Clips, clip)
				timelinePos += dur
			}

			seq.Tracks = append(seq.Tracks, track)
			if err := s.store.UpdateSequence(projectID, seq); err != nil {
				internalError(w, "failed to add clips to sequence: "+seqReq.Name)
				return
			}
		}

		results = append(results, seq)
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"sequences": results,
		"count":     len(results),
	})
}

func (s *Server) handleListSequences(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	seqs, err := s.store.ListSequences(projectID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, "project not found")
			return
		}
		internalError(w, "failed to list sequences")
		return
	}
	if seqs == nil {
		seqs = []model.Sequence{}
	}
	writeJSON(w, http.StatusOK, seqs)
}

func (s *Server) handleGetSequence(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, seq)
}

type updateSequenceRequest struct {
	Name            *string      `json:"name,omitempty"`
	IntroSlide      *model.Slide `json:"intro_slide,omitempty"`
	OutroSlide      *model.Slide `json:"outro_slide,omitempty"`
	SubtitleAssetID *string      `json:"subtitle_asset_id,omitempty"`
}

func (s *Server) handleUpdateSequence(w http.ResponseWriter, r *http.Request) {
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

	var req updateSequenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	if req.Name != nil {
		seq.Name = *req.Name
	}
	if req.IntroSlide != nil {
		seq.IntroSlide = req.IntroSlide
	}
	if req.OutroSlide != nil {
		seq.OutroSlide = req.OutroSlide
	}
	if req.SubtitleAssetID != nil {
		seq.SubtitleAssetID = *req.SubtitleAssetID
	}

	if err := s.store.UpdateSequence(projectID, seq); err != nil {
		internalError(w, "failed to update sequence")
		return
	}
	writeJSONWithViz(w, http.StatusOK, seq, projectID, seq.ID)
}

func (s *Server) handleDeleteSequence(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	seqID := chi.URLParam(r, "seqID")
	if err := s.store.DeleteSequence(projectID, seqID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, err.Error())
			return
		}
		internalError(w, "failed to delete sequence")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Tracks

type createTrackRequest struct {
	Name    string          `json:"name"`
	Type    model.TrackType `json:"type"`
	Index   int             `json:"index"`
	Muted   bool            `json:"muted"`
	Locked  bool            `json:"locked"`
	Opacity *float64        `json:"opacity,omitempty"`
	Volume  *float64        `json:"volume,omitempty"`
}

func (s *Server) handleCreateTrack(w http.ResponseWriter, r *http.Request) {
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

	var req createTrackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if req.Name == "" {
		badRequest(w, "name is required")
		return
	}
	if req.Type != model.TrackTypeVideo && req.Type != model.TrackTypeAudio {
		badRequest(w, "type must be 'video' or 'audio'")
		return
	}

	opacity := 1.0
	if req.Opacity != nil {
		opacity = *req.Opacity
	}
	volume := 1.0
	if req.Volume != nil {
		volume = *req.Volume
	}

	track := model.Track{
		ID:      uuid.New().String(),
		Name:    req.Name,
		Type:    req.Type,
		Index:   req.Index,
		Muted:   req.Muted,
		Locked:  req.Locked,
		Opacity: opacity,
		Volume:  volume,
	}

	seq.Tracks = append(seq.Tracks, track)
	if err := s.store.UpdateSequence(projectID, seq); err != nil {
		internalError(w, "failed to add track")
		return
	}
	writeJSONWithViz(w, http.StatusCreated, track, projectID, seqID)
}

func (s *Server) handleUpdateTrack(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	seqID := chi.URLParam(r, "seqID")
	trackID := chi.URLParam(r, "trackID")

	seq, err := s.store.GetSequence(projectID, seqID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, err.Error())
			return
		}
		internalError(w, "failed to get sequence")
		return
	}

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	found := false
	for i := range seq.Tracks {
		if seq.Tracks[i].ID == trackID {
			found = true
			if v, ok := updates["name"].(string); ok {
				seq.Tracks[i].Name = v
			}
			if v, ok := updates["muted"].(bool); ok {
				seq.Tracks[i].Muted = v
			}
			if v, ok := updates["locked"].(bool); ok {
				seq.Tracks[i].Locked = v
			}
			if v, ok := updates["opacity"].(float64); ok {
				seq.Tracks[i].Opacity = v
			}
			if v, ok := updates["volume"].(float64); ok {
				seq.Tracks[i].Volume = v
			}
			if v, ok := updates["index"].(float64); ok {
				seq.Tracks[i].Index = int(v)
			}
			break
		}
	}
	if !found {
		notFound(w, "track not found")
		return
	}

	if err := s.store.UpdateSequence(projectID, seq); err != nil {
		internalError(w, "failed to update track")
		return
	}
	writeJSONWithViz(w, http.StatusOK, seq, projectID, seqID)
}

func (s *Server) handleDeleteTrack(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	seqID := chi.URLParam(r, "seqID")
	trackID := chi.URLParam(r, "trackID")

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
	tracks := make([]model.Track, 0, len(seq.Tracks))
	for _, t := range seq.Tracks {
		if t.ID == trackID {
			found = true
			continue
		}
		tracks = append(tracks, t)
	}
	if !found {
		notFound(w, "track not found")
		return
	}

	seq.Tracks = tracks
	if err := s.store.UpdateSequence(projectID, seq); err != nil {
		internalError(w, "failed to delete track")
		return
	}
	writeJSONWithViz(w, http.StatusOK, seq, projectID, seqID)
}

// Clips

type createClipRequest struct {
	AssetID       string           `json:"asset_id"`
	TimelineIn    float64          `json:"timeline_in"`
	TimelineOut   float64          `json:"timeline_out"`
	SourceIn      float64          `json:"source_in"`
	SourceOut     float64          `json:"source_out"`
	Speed         float64          `json:"speed"`
	Enabled       *bool            `json:"enabled,omitempty"`
	TransitionIn  *model.Transition `json:"transition_in,omitempty"`
	TransitionOut *model.Transition `json:"transition_out,omitempty"`
}

func (s *Server) handleCreateClip(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	seqID := chi.URLParam(r, "seqID")
	trackID := chi.URLParam(r, "trackID")

	seq, err := s.store.GetSequence(projectID, seqID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, err.Error())
			return
		}
		internalError(w, "failed to get sequence")
		return
	}

	var req createClipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	speed := req.Speed
	if speed == 0 {
		speed = 1.0
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	clip := model.Clip{
		ID:            uuid.New().String(),
		TrackID:       trackID,
		AssetID:       req.AssetID,
		TimelineIn:    req.TimelineIn,
		TimelineOut:   req.TimelineOut,
		SourceIn:      req.SourceIn,
		SourceOut:     req.SourceOut,
		Speed:         speed,
		Enabled:       enabled,
		TransitionIn:  req.TransitionIn,
		TransitionOut: req.TransitionOut,
	}

	if err := clip.Validate(); err != nil {
		badRequest(w, err.Error())
		return
	}

	found := false
	for i := range seq.Tracks {
		if seq.Tracks[i].ID == trackID {
			found = true
			seq.Tracks[i].Clips = append(seq.Tracks[i].Clips, clip)
			break
		}
	}
	if !found {
		notFound(w, "track not found")
		return
	}

	if err := s.store.UpdateSequence(projectID, seq); err != nil {
		internalError(w, "failed to add clip")
		return
	}

	// Auto-generate thumbnail for this clip
	go s.autoGenerateThumbnails(projectID, seq)

	// Include inline thumbnail in response
	response := map[string]interface{}{
		"clip": clip,
	}
	if asset, err := s.store.GetAsset(projectID, req.AssetID); err == nil && asset.Type == model.AssetTypeVideo {
		if thumb := extractFrameBase64(asset.FilePath, req.SourceIn); thumb != "" {
			response["thumbnail"] = thumb
		}
	}

	writeJSONWithViz(w, http.StatusCreated, response, projectID, seqID)
}

func (s *Server) handleUpdateClip(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	seqID := chi.URLParam(r, "seqID")
	trackID := chi.URLParam(r, "trackID")
	clipID := chi.URLParam(r, "clipID")

	seq, err := s.store.GetSequence(projectID, seqID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, err.Error())
			return
		}
		internalError(w, "failed to get sequence")
		return
	}

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	found := false
	for i := range seq.Tracks {
		if seq.Tracks[i].ID != trackID {
			continue
		}
		for j := range seq.Tracks[i].Clips {
			if seq.Tracks[i].Clips[j].ID == clipID {
				found = true
				c := &seq.Tracks[i].Clips[j]
				if v, ok := updates["timeline_in"].(float64); ok {
					c.TimelineIn = v
				}
				if v, ok := updates["timeline_out"].(float64); ok {
					c.TimelineOut = v
				}
				if v, ok := updates["source_in"].(float64); ok {
					c.SourceIn = v
				}
				if v, ok := updates["source_out"].(float64); ok {
					c.SourceOut = v
				}
				if v, ok := updates["speed"].(float64); ok {
					c.Speed = v
				}
				if v, ok := updates["enabled"].(bool); ok {
					c.Enabled = v
				}
				// Crop settings
				if v, ok := updates["crop"]; ok {
					if v == nil {
						c.Crop = nil
					} else {
						data, _ := json.Marshal(v)
						var crop model.CropSettings
						if err := json.Unmarshal(data, &crop); err == nil {
							c.Crop = &crop
						}
					}
				}
				// Transition in
				if v, ok := updates["transition_in"]; ok {
					if v == nil {
						c.TransitionIn = nil
					} else {
						data, _ := json.Marshal(v)
						var t model.Transition
						if err := json.Unmarshal(data, &t); err == nil {
							c.TransitionIn = &t
						}
					}
				}
				// Transition out
				if v, ok := updates["transition_out"]; ok {
					if v == nil {
						c.TransitionOut = nil
					} else {
						data, _ := json.Marshal(v)
						var t model.Transition
						if err := json.Unmarshal(data, &t); err == nil {
							c.TransitionOut = &t
						}
					}
				}
				break
			}
		}
		break
	}
	if !found {
		notFound(w, "clip not found")
		return
	}

	if err := s.store.UpdateSequence(projectID, seq); err != nil {
		internalError(w, "failed to update clip")
		return
	}
	writeJSONWithViz(w, http.StatusOK, seq, projectID, seqID)
}

func (s *Server) handleDeleteClip(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	seqID := chi.URLParam(r, "seqID")
	trackID := chi.URLParam(r, "trackID")
	clipID := chi.URLParam(r, "clipID")

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
	for i := range seq.Tracks {
		if seq.Tracks[i].ID != trackID {
			continue
		}
		clips := make([]model.Clip, 0, len(seq.Tracks[i].Clips))
		for _, c := range seq.Tracks[i].Clips {
			if c.ID == clipID {
				found = true
				continue
			}
			clips = append(clips, c)
		}
		seq.Tracks[i].Clips = clips
		break
	}
	if !found {
		notFound(w, "clip not found")
		return
	}

	if err := s.store.UpdateSequence(projectID, seq); err != nil {
		internalError(w, "failed to delete clip")
		return
	}
	writeJSONWithViz(w, http.StatusOK, seq, projectID, seqID)
}
