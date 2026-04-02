package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"video-editor/internal/engine"
	"video-editor/internal/model"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type importAssetRequest struct {
	Name     string `json:"name"`
	FilePath string `json:"file_path"`
	Type     string `json:"type,omitempty"`
}

func (s *Server) handleImportAsset(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	// Check project exists
	if _, err := s.store.GetProject(projectID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, "project not found")
			return
		}
		internalError(w, "failed to get project")
		return
	}

	contentType := r.Header.Get("Content-Type")

	var asset *model.Asset
	var err error

	if strings.HasPrefix(contentType, "multipart/form-data") {
		asset, err = s.handleMultipartImport(r, projectID)
	} else {
		asset, err = s.handleJSONImport(r, projectID)
	}

	if err != nil {
		if strings.HasPrefix(err.Error(), "bad:") {
			badRequest(w, strings.TrimPrefix(err.Error(), "bad:"))
			return
		}
		internalError(w, err.Error())
		return
	}

	// Probe metadata
	meta, probeErr := probeFile(asset.FilePath)
	if probeErr == nil {
		asset.Metadata = *meta
	}

	// Auto-detect type if not specified
	if asset.Type == "" {
		asset.Type = detectAssetType(asset.FilePath, meta)
	}

	// Auto-detect Zoom view type and recording group from original filename
	if asset.ViewType == "" {
		asset.ViewType = engine.DetectViewType(asset.Name)
		if asset.ViewType == "" {
			// Try the original file path too
			asset.ViewType = engine.DetectViewType(asset.FilePath)
		}
	}
	if asset.RecordingGroup == "" {
		asset.RecordingGroup = engine.ExtractRecordingGroup(asset.Name)
		if asset.RecordingGroup == "" {
			asset.RecordingGroup = engine.ExtractRecordingGroup(asset.FilePath)
		}
	}

	asset.ProxyStatus = model.ProxyStatusNone
	asset.CreatedAt = time.Now().UTC()

	if err := asset.Validate(); err != nil {
		badRequest(w, err.Error())
		return
	}

	if err := s.store.CreateAsset(projectID, asset); err != nil {
		internalError(w, "failed to create asset")
		return
	}

	// Generate inline thumbnail for video assets
	response := map[string]interface{}{
		"asset": asset,
	}
	if asset.Type == model.AssetTypeVideo && asset.Metadata.Duration > 0 {
		// Grab a frame at 10% into the video
		thumbTime := asset.Metadata.Duration * 0.1
		if thumb := extractFrameBase64(asset.FilePath, thumbTime); thumb != "" {
			response["thumbnail"] = thumb
		}
	}

	writeJSONWithViz(w, http.StatusCreated, response, projectID, "")
}

func (s *Server) handleJSONImport(r *http.Request, projectID string) (*model.Asset, error) {
	var req importAssetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, fmt.Errorf("bad:invalid JSON body")
	}
	if req.Name == "" {
		return nil, fmt.Errorf("bad:name is required")
	}
	if req.FilePath == "" {
		return nil, fmt.Errorf("bad:file_path is required")
	}

	// Verify file exists
	info, err := os.Stat(req.FilePath)
	if err != nil {
		return nil, fmt.Errorf("bad:file not found: %s", req.FilePath)
	}

	assetID := uuid.New().String()
	ext := filepath.Ext(req.FilePath)
	destPath := s.store.AssetOriginalPath(projectID, assetID, ext)

	// Copy file to asset storage
	if err := copyFile(req.FilePath, destPath); err != nil {
		return nil, fmt.Errorf("failed to copy asset file: %w", err)
	}

	return &model.Asset{
		ID:        assetID,
		ProjectID: projectID,
		Name:      req.Name,
		FilePath:  destPath,
		Type:      model.AssetType(req.Type),
		Metadata:  model.AssetMetadata{FileSize: info.Size()},
	}, nil
}

func (s *Server) handleMultipartImport(r *http.Request, projectID string) (*model.Asset, error) {
	if err := r.ParseMultipartForm(500 << 20); err != nil { // 500MB max
		return nil, fmt.Errorf("bad:failed to parse multipart form: %v", err)
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("bad:file field is required")
	}
	defer file.Close()

	name := r.FormValue("name")
	if name == "" {
		name = header.Filename
	}
	assetType := r.FormValue("type")

	assetID := uuid.New().String()
	ext := filepath.Ext(header.Filename)
	destPath := s.store.AssetOriginalPath(projectID, assetID, ext)

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create asset directory: %w", err)
	}

	dst, err := os.Create(destPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		return nil, fmt.Errorf("failed to write asset file: %w", err)
	}

	return &model.Asset{
		ID:        assetID,
		ProjectID: projectID,
		Name:      name,
		FilePath:  destPath,
		Type:      model.AssetType(assetType),
		Metadata:  model.AssetMetadata{FileSize: written},
	}, nil
}

func (s *Server) handleListAssets(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	typeFilter := r.URL.Query().Get("type")

	var assetType *model.AssetType
	if typeFilter != "" {
		t := model.AssetType(typeFilter)
		assetType = &t
	}

	assets, err := s.store.ListAssets(projectID, assetType)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, "project not found")
			return
		}
		internalError(w, "failed to list assets")
		return
	}
	if assets == nil {
		assets = []model.Asset{}
	}
	writeJSON(w, http.StatusOK, assets)
}

func (s *Server) handleGetAsset(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	assetID := chi.URLParam(r, "assetID")

	asset, err := s.store.GetAsset(projectID, assetID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, "asset not found")
			return
		}
		internalError(w, "failed to get asset")
		return
	}
	writeJSON(w, http.StatusOK, asset)
}

func (s *Server) handleDeleteAsset(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	assetID := chi.URLParam(r, "assetID")

	if err := s.store.DeleteAsset(projectID, assetID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			notFound(w, err.Error())
			return
		}
		internalError(w, "failed to delete asset")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ffprobe metadata extraction

type ffprobeOutput struct {
	Streams []ffprobeStream `json:"streams"`
	Format  ffprobeFormat   `json:"format"`
}

type ffprobeStream struct {
	CodecType    string `json:"codec_type"`
	CodecName    string `json:"codec_name"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	PixFmt       string `json:"pix_fmt"`
	SampleRate   string `json:"sample_rate"`
	Channels     int    `json:"channels"`
	Duration     string `json:"duration"`
	BitRate      string `json:"bit_rate"`
}

type ffprobeFormat struct {
	Duration string `json:"duration"`
	Size     string `json:"size"`
	BitRate  string `json:"bit_rate"`
}

func probeFile(path string) (*model.AssetMetadata, error) {
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	var probe ffprobeOutput
	if err := json.Unmarshal(out, &probe); err != nil {
		return nil, fmt.Errorf("parse ffprobe output: %w", err)
	}

	meta := &model.AssetMetadata{}

	// Parse format-level info
	if d, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil {
		meta.Duration = d
	}
	if s, err := strconv.ParseInt(probe.Format.Size, 10, 64); err == nil {
		meta.FileSize = s
	}
	if br, err := strconv.ParseInt(probe.Format.BitRate, 10, 64); err == nil {
		meta.BitRate = br
	}

	// Parse stream-level info — find first video and first audio stream
	// regardless of stream order (Zoom recordings often have audio as stream 0)
	videoFound := false
	audioFound := false
	for _, stream := range probe.Streams {
		switch stream.CodecType {
		case "video":
			if !videoFound {
				meta.Codec = stream.CodecName
				meta.Width = stream.Width
				meta.Height = stream.Height
				meta.PixelFormat = stream.PixFmt
				videoFound = true
			}
		case "audio":
			if !audioFound {
				if sr, err := strconv.Atoi(stream.SampleRate); err == nil {
					meta.SampleRate = sr
				}
				meta.Channels = stream.Channels
				meta.AudioCodec = stream.CodecName
				audioFound = true
			}
		}
	}
	// For audio-only files, use audio codec as primary codec
	if !videoFound && audioFound {
		meta.Codec = meta.AudioCodec
	}

	return meta, nil
}

func detectAssetType(path string, meta *model.AssetMetadata) model.AssetType {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp4", ".mov", ".avi", ".mkv", ".webm", ".wmv", ".flv":
		return model.AssetTypeVideo
	case ".mp3", ".wav", ".aac", ".flac", ".ogg", ".m4a":
		return model.AssetTypeAudio
	case ".jpg", ".jpeg", ".png", ".bmp", ".tiff", ".gif", ".webp":
		return model.AssetTypeImage
	case ".srt", ".ass", ".vtt":
		return model.AssetTypeSubtitle
	}

	// Fallback to ffprobe info
	if meta != nil {
		if meta.Width > 0 {
			return model.AssetTypeVideo
		}
		if meta.SampleRate > 0 {
			return model.AssetTypeAudio
		}
	}

	return model.AssetTypeVideo
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
