package api

import (
	"net/http"
	"os/exec"
	"runtime"
	"strings"

	"video-editor/internal/engine"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ffmpegVersion := "unknown"
	out, err := exec.Command("ffmpeg", "-version").Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) > 0 {
			ffmpegVersion = strings.TrimSpace(lines[0])
		}
	}

	whisperVersion := engine.DetectWhisper()
	gpuAvailable := engine.DetectGPU()

	result := map[string]interface{}{
		"status":         "ok",
		"ffmpeg_version": ffmpegVersion,
		"go_version":     runtime.Version(),
	}

	if whisperVersion != "" {
		result["whisper_version"] = whisperVersion
		result["whisper_available"] = true
	} else {
		result["whisper_available"] = false
	}

	result["gpu_available"] = gpuAvailable
	result["whisper_model"] = s.whisperModel

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	features := []string{
		"projects",
		"assets",
		"asset_import",
		"ffprobe_metadata",
		"overlays",
		"text_overlays",
		"transitions",
		"crop",
		"subtitle_styling",
	}

	whisperVersion := engine.DetectWhisper()
	if whisperVersion != "" {
		features = append(features, "transcription")
	}
	if engine.DetectGPU() {
		features = append(features, "gpu_acceleration")
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":  "0.2.0",
		"features": features,
		"supported_formats": map[string][]string{
			"video": {"mp4", "mov", "avi", "mkv", "webm"},
			"audio": {"mp3", "wav", "aac", "flac", "ogg"},
			"image": {"jpg", "jpeg", "png", "bmp", "tiff"},
		},
	})
}
