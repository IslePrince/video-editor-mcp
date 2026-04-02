package api

import (
	"net/http"
	"strconv"
	"strings"

	"video-editor/internal/engine"
	"video-editor/internal/model"

	"github.com/go-chi/chi/v5"
)

type suggestClipResponse struct {
	Title             string  `json:"title"`
	StartTime         float64 `json:"start_time"`
	EndTime           float64 `json:"end_time"`
	TranscriptPreview string  `json:"transcript_preview"`
	Reason            string  `json:"reason"`
	Score             float64 `json:"score"`
	Thumbnail         string  `json:"thumbnail,omitempty"`
}

func (s *Server) handleSuggestClips(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	assetID := chi.URLParam(r, "assetID")

	asset, err := s.store.GetAsset(projectID, assetID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, err.Error())
			return
		}
		internalError(w, "failed to get asset")
		return
	}

	maxClips := 5
	if v := r.URL.Query().Get("max"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxClips = n
		}
	}
	minDuration := 15.0
	if v := r.URL.Query().Get("min_duration"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
			minDuration = n
		}
	}
	maxDuration := 120.0
	if v := r.URL.Query().Get("max_duration"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
			maxDuration = n
		}
	}
	topic := r.URL.Query().Get("topic")

	entries, err := engine.ParseVTT(asset.FilePath)
	if err != nil {
		internalError(w, "failed to parse transcript: "+err.Error())
		return
	}

	// Filter by topic if specified
	if topic != "" {
		entries = filterByTopic(entries, topic)
	}

	suggestions := engine.SuggestClips(entries, maxClips, minDuration, maxDuration)

	// Try to find a video asset for thumbnail extraction
	videoFilePath := ""
	if vid := r.URL.Query().Get("video_asset_id"); vid != "" {
		if va, err := s.store.GetAsset(projectID, vid); err == nil {
			videoFilePath = va.FilePath
		}
	} else {
		// Try to find the first video asset in the project
		videoType := model.AssetTypeVideo
		assets, _ := s.store.ListAssets(projectID, &videoType)
		if len(assets) > 0 {
			videoFilePath = assets[0].FilePath
		}
	}

	// Build response with optional thumbnails
	results := make([]suggestClipResponse, len(suggestions))
	for i, sg := range suggestions {
		results[i] = suggestClipResponse{
			Title:             sg.Title,
			StartTime:         sg.StartTime,
			EndTime:           sg.EndTime,
			TranscriptPreview: sg.TranscriptPreview,
			Reason:            sg.Reason,
			Score:             sg.Score,
		}
		if videoFilePath != "" {
			results[i].Thumbnail = extractFrameBase64(videoFilePath, sg.StartTime)
		}
	}

	writeJSON(w, http.StatusOK, results)
}

// filterByTopic returns only entries whose text mentions the topic keywords.
func filterByTopic(entries []engine.SubtitleEntry, topic string) []engine.SubtitleEntry {
	keywords := strings.Fields(strings.ToLower(topic))
	if len(keywords) == 0 {
		return entries
	}

	var filtered []engine.SubtitleEntry
	for _, e := range entries {
		lower := strings.ToLower(e.Text)
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				filtered = append(filtered, e)
				break
			}
		}
	}

	// If filtering removed too much content, return all entries
	if len(filtered) < 5 {
		return entries
	}
	return filtered
}
