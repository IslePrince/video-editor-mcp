package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"video-editor/internal/engine"
	"video-editor/internal/model"

	"github.com/go-chi/chi/v5"
)

type mediaFileInfo struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	Size           int64  `json:"size"`
	SizeMB         string `json:"size_mb"`
	Type           string `json:"type"`
	RecordingGroup string `json:"recording_group,omitempty"`
	View           string `json:"view,omitempty"`
	Resolution     string `json:"resolution,omitempty"`
	Description    string `json:"description,omitempty"`
}

// handleListMediaFiles scans the /media directory and returns available files.
func (s *Server) handleListMediaFiles(w http.ResponseWriter, r *http.Request) {
	mediaDir := s.mediaPath
	if mediaDir == "" {
		mediaDir = "/media"
	}

	entries, err := os.ReadDir(mediaDir)
	if err != nil {
		writeJSON(w, http.StatusOK, []mediaFileInfo{})
		return
	}

	var files []mediaFileInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		fileType := detectFileType(ext)

		mfi := mediaFileInfo{
			Name:   name,
			Path:   filepath.Join(mediaDir, name),
			Size:   info.Size(),
			SizeMB: fmt.Sprintf("%.1f", float64(info.Size())/1024/1024),
			Type:   fileType,
		}

		// Detect Zoom recording metadata
		mfi.View = engine.DetectViewType(name)
		mfi.RecordingGroup = engine.ExtractRecordingGroup(name)
		if mfi.View != "" {
			mfi.Description = viewDescription(mfi.View)
		}

		// Probe resolution for video files
		if fileType == "video" {
			if res := probeResolution(filepath.Join(mediaDir, name)); res != "" {
				mfi.Resolution = res
			}
		}

		files = append(files, mfi)
	}

	if files == nil {
		files = []mediaFileInfo{}
	}
	writeJSON(w, http.StatusOK, files)
}

func detectFileType(ext string) string {
	switch ext {
	case ".mp4", ".mov", ".avi", ".mkv", ".webm":
		return "video"
	case ".mp3", ".wav", ".aac", ".m4a", ".flac", ".ogg":
		return "audio"
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".svg", ".tiff", ".tif":
		return "image"
	case ".vtt", ".srt", ".ass":
		return "subtitle"
	case ".txt", ".json", ".csv":
		return "text"
	default:
		return "other"
	}
}

func viewDescription(view string) string {
	switch view {
	case "speaker":
		return "Active speaker view — camera follows whoever is talking. Best for social media clips focused on one person speaking."
	case "gallery":
		return "Gallery view — all participants visible side by side. Best for group discussions and reaction shots."
	case "active_video":
		return "Active speaker video only (low-res). Speaker-focused but lower quality than the main speaker view."
	case "combined":
		return "Combined/default view. Contains all participants in the standard Zoom layout."
	default:
		return ""
	}
}

func probeResolution(filePath string) string {
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "csv=p=0",
		filePath,
	)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	res := strings.TrimSpace(string(out))
	// ffprobe returns "width,height" — convert to "widthxheight"
	parts := strings.Split(res, ",")
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]) + "x" + strings.TrimSpace(parts[1])
	}
	return ""
}

// handleReadTranscript reads a VTT/SRT file and returns the text content.
// Supports optional start/end query params to filter by time range.
func (s *Server) handleReadTranscript(w http.ResponseWriter, r *http.Request) {
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

	data, err := os.ReadFile(asset.FilePath)
	if err != nil {
		internalError(w, "failed to read transcript file")
		return
	}

	// Optional time range filter
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	content := string(data)

	if startStr != "" || endStr != "" {
		startSec := 0.0
		endSec := 999999.0
		if startStr != "" {
			startSec, _ = strconv.ParseFloat(startStr, 64)
		}
		if endStr != "" {
			endSec, _ = strconv.ParseFloat(endStr, 64)
		}
		content = filterVTTByTime(content, startSec, endSec)
	}

	result := map[string]interface{}{
		"asset_id": assetID,
		"name":     asset.Name,
		"content":  content,
	}
	writeJSON(w, http.StatusOK, result)
}

// filterVTTByTime returns only VTT entries within the time range
func filterVTTByTime(vtt string, startSec, endSec float64) string {
	lines := strings.Split(vtt, "\n")
	var result []string
	var inRange bool
	var currentBlock []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for timestamp line
		if strings.Contains(trimmed, "-->") {
			parts := strings.Split(trimmed, "-->")
			if len(parts) == 2 {
				entryStart := parseVTTTime(strings.TrimSpace(parts[0]))
				entryEnd := parseVTTTime(strings.TrimSpace(parts[1]))
				inRange = entryEnd >= startSec && entryStart <= endSec
				if inRange {
					currentBlock = append(currentBlock, line)
				}
				continue
			}
		}

		if inRange && trimmed != "" {
			currentBlock = append(currentBlock, line)
		} else if trimmed == "" && len(currentBlock) > 0 {
			result = append(result, strings.Join(currentBlock, "\n"))
			currentBlock = nil
			inRange = false
		}
	}
	if len(currentBlock) > 0 {
		result = append(result, strings.Join(currentBlock, "\n"))
	}

	return strings.Join(result, "\n\n")
}

func parseVTTTime(tc string) float64 {
	// Parse HH:MM:SS.mmm or MM:SS.mmm
	parts := strings.Split(tc, ":")
	switch len(parts) {
	case 3:
		// HH:MM:SS.mmm
		h, _ := strconv.ParseFloat(parts[0], 64)
		m, _ := strconv.ParseFloat(parts[1], 64)
		sParts := strings.Split(parts[2], ".")
		sec, _ := strconv.ParseFloat(sParts[0], 64)
		ms := 0.0
		if len(sParts) > 1 {
			ms, _ = strconv.ParseFloat("0."+sParts[1], 64)
		}
		return h*3600 + m*60 + sec + ms
	case 2:
		// MM:SS.mmm
		m, _ := strconv.ParseFloat(parts[0], 64)
		sParts := strings.Split(parts[1], ".")
		sec, _ := strconv.ParseFloat(sParts[0], 64)
		ms := 0.0
		if len(sParts) > 1 {
			ms, _ = strconv.ParseFloat("0."+sParts[1], 64)
		}
		return m*60 + sec + ms
	}
	return 0
}

// handlePreviewFrame extracts a single frame from an asset at a given timestamp.
func (s *Server) handlePreviewFrame(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	assetID := chi.URLParam(r, "assetID")
	timeStr := r.URL.Query().Get("time")

	if timeStr == "" {
		badRequest(w, "time query parameter required (in seconds)")
		return
	}

	asset, err := s.store.GetAsset(projectID, assetID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, err.Error())
			return
		}
		internalError(w, "failed to get asset")
		return
	}

	// Extract frame using ffmpeg — explicitly select video stream to handle
	// files where audio is stream 0 (common with Zoom recordings)
	cmd := exec.Command("ffmpeg",
		"-ss", timeStr,
		"-i", asset.FilePath,
		"-map", "0:v:0",
		"-vframes", "1",
		"-f", "image2",
		"-c:v", "mjpeg",
		"-q:v", "3",
		"pipe:1",
	)

	var stderr strings.Builder
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = err.Error()
		}
		// Check for common issues and provide helpful messages
		if asset.Metadata.Width == 0 || asset.Metadata.Height == 0 {
			internalError(w, "failed to extract frame: no video stream detected in asset metadata. Try re-importing the file. ffmpeg: "+errMsg)
		} else {
			internalError(w, "failed to extract frame: "+errMsg)
		}
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(output)
}

// handleAddClipsBatch adds multiple clips to a track in a single request.
func (s *Server) handleAddClipsBatch(w http.ResponseWriter, r *http.Request) {
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

	var clips []createClipRequest
	if err := json.NewDecoder(r.Body).Decode(&clips); err != nil {
		badRequest(w, "expected JSON array of clip objects")
		return
	}

	trackIdx := -1
	for i := range seq.Tracks {
		if seq.Tracks[i].ID == trackID {
			trackIdx = i
			break
		}
	}
	if trackIdx < 0 {
		notFound(w, "track not found")
		return
	}

	var added []map[string]interface{}
	for _, req := range clips {
		speed := req.Speed
		if speed == 0 {
			speed = 1.0
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}

		clip := newClipFromRequest(req, trackID, speed, enabled)
		if err := clip.Validate(); err != nil {
			badRequest(w, fmt.Sprintf("clip validation failed: %s", err.Error()))
			return
		}

		seq.Tracks[trackIdx].Clips = append(seq.Tracks[trackIdx].Clips, clip)
		added = append(added, map[string]interface{}{
			"id":           clip.ID,
			"timeline_in":  clip.TimelineIn,
			"timeline_out": clip.TimelineOut,
			"source_in":    clip.SourceIn,
			"source_out":   clip.SourceOut,
		})
	}

	if err := s.store.UpdateSequence(projectID, seq); err != nil {
		internalError(w, "failed to save clips")
		return
	}

	// Auto-generate thumbnails for new clips
	s.autoGenerateThumbnails(projectID, seq)

	result := map[string]interface{}{
		"clips_added": len(added),
		"clips":       added,
	}
	writeJSONWithViz(w, http.StatusCreated, result, projectID, seqID)
}

func (s *Server) autoGenerateThumbnails(projectID string, seq *model.Sequence) {
	for _, track := range seq.Tracks {
		if track.Type != model.TrackTypeVideo || track.Muted {
			continue
		}
		for _, clip := range track.Clips {
			if !clip.Enabled {
				continue
			}
			asset, err := s.store.GetAsset(projectID, clip.AssetID)
			if err != nil {
				continue
			}
			thumbDir := s.store.ThumbnailsPath(projectID)
			thumbPath := filepath.Join(thumbDir, clip.ID+".jpg")
			if _, err := os.Stat(thumbPath); err == nil {
				continue // already exists
			}
			os.MkdirAll(thumbDir, 0755)
			midpoint := (clip.SourceIn + clip.SourceOut) / 2
			cmd := exec.Command("ffmpeg", "-y",
				"-ss", fmt.Sprintf("%g", midpoint),
				"-i", asset.FilePath,
				"-vframes", "1",
				"-q:v", "2",
				thumbPath,
			)
			cmd.Run()
		}
	}
}
