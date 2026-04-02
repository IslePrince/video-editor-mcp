package engine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"video-editor/internal/model"
)

type ClipThumbnail struct {
	ClipID   string `json:"clip_id"`
	Filename string `json:"filename"`
}

// GenerateClipThumbnail extracts a single frame from a video at the given time.
func GenerateClipThumbnail(assetPath string, timeSeconds float64, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("create thumbnail dir: %w", err)
	}

	cmd := exec.Command("ffmpeg", "-y",
		"-ss", fmt.Sprintf("%g", timeSeconds),
		"-i", assetPath,
		"-vframes", "1",
		"-q:v", "2",
		outputPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg thumbnail failed: %w\n%s", err, string(out))
	}
	return nil
}

// GenerateSequenceThumbnails creates a thumbnail for each clip in the sequence.
// Returns a list of clip IDs with their thumbnail filenames.
func GenerateSequenceThumbnails(seq *model.Sequence, assets map[string]*model.Asset, thumbnailDir string) ([]ClipThumbnail, error) {
	var clips []model.Clip
	for _, track := range seq.Tracks {
		if track.Type != model.TrackTypeVideo || track.Muted {
			continue
		}
		for _, clip := range track.Clips {
			if clip.Enabled {
				clips = append(clips, clip)
			}
		}
	}
	sort.Slice(clips, func(i, j int) bool {
		return clips[i].TimelineIn < clips[j].TimelineIn
	})

	var results []ClipThumbnail
	for _, clip := range clips {
		asset, ok := assets[clip.AssetID]
		if !ok {
			continue
		}

		midpoint := (clip.SourceIn + clip.SourceOut) / 2
		filename := clip.ID + ".jpg"
		outputPath := filepath.Join(thumbnailDir, filename)

		if err := GenerateClipThumbnail(asset.FilePath, midpoint, outputPath); err != nil {
			// Log but continue
			continue
		}

		results = append(results, ClipThumbnail{
			ClipID:   clip.ID,
			Filename: filename,
		})
	}

	return results, nil
}
