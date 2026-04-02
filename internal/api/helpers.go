package api

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"

	"video-editor/internal/model"

	"github.com/google/uuid"
)

func newClipFromRequest(req createClipRequest, trackID string, speed float64, enabled bool) model.Clip {
	return model.Clip{
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
}

// extractFrameBase64 extracts a single JPEG frame at the given timestamp and
// returns it as a base64-encoded data URI. Returns empty string on failure.
func extractFrameBase64(filePath string, timeSec float64) string {
	cmd := exec.Command("ffmpeg",
		"-ss", fmt.Sprintf("%.3f", timeSec),
		"-i", filePath,
		"-map", "0:v:0",
		"-frames:v", "1",
		"-f", "image2",
		"-c:v", "mjpeg",
		"-q:v", "5", // lower quality for thumbnails (smaller payload)
		"-vf", "scale=320:-1", // thumbnail size
		"pipe:1",
	)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return ""
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(out)
}
