package engine

import (
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"os"
	"path/filepath"

	"video-editor/internal/model"
)

// RenderTimelineImage generates an NLE-style timeline visualization.
func RenderTimelineImage(seq *model.Sequence, assets map[string]*model.Asset, thumbnailDir string) (*image.RGBA, error) {
	// Collect tracks sorted by type then index (video first, then audio)
	type trackInfo struct {
		track model.Track
		label string
	}
	var videoTracks, audioTracks []trackInfo
	vIdx, aIdx := 1, 1
	for _, t := range seq.Tracks {
		if t.Type == model.TrackTypeVideo {
			videoTracks = append(videoTracks, trackInfo{track: t, label: fmt.Sprintf("V%d", vIdx)})
			vIdx++
		} else {
			audioTracks = append(audioTracks, trackInfo{track: t, label: fmt.Sprintf("A%d", aIdx)})
			aIdx++
		}
	}
	allTracks := append(videoTracks, audioTracks...)
	if len(allTracks) == 0 {
		// Return empty state
		return renderEmptyTimeline("No tracks in sequence"), nil
	}

	// Compute total duration
	totalDuration := computeTotalDuration(seq)
	if totalDuration <= 0 {
		totalDuration = 10 // minimum 10s for empty timeline
	}

	metrics := NewLayoutMetrics(totalDuration, len(allTracks))
	img := image.NewRGBA(image.Rect(0, 0, metrics.CanvasWidth, metrics.CanvasHeight))

	// Fill background
	DrawRect(img, 0, 0, metrics.CanvasWidth, metrics.CanvasHeight, ColorBackground)

	// Draw timecode ruler
	drawRuler(img, metrics)

	// Draw each track
	for i, ti := range allTracks {
		y := metrics.TrackY(i)

		// Track lane background (alternating)
		laneColor := ColorTrackLane
		if i%2 == 1 {
			laneColor = ColorTrackAlt
		}
		DrawRect(img, 0, y, metrics.CanvasWidth, TrackLaneHeight, laneColor)

		// Track label
		labelY := y + TrackLaneHeight/2 + 5
		DrawText(img, 6, labelY, ti.label, ColorText)
		if ti.track.Muted {
			DrawText(img, 6, labelY+14, "M", ColorTextDim)
		}

		// Draw clips
		for _, clip := range ti.track.Clips {
			if !clip.Enabled {
				continue
			}
			drawClipBlock(img, metrics, clip, ti.track.Type, assets, thumbnailDir, y)
		}
	}

	// Draw intro/outro slide blocks on first video track
	if len(videoTracks) > 0 {
		y := metrics.TrackY(0)
		timeOffset := 0.0

		if seq.IntroSlide != nil {
			x1 := metrics.XForTime(0)
			x2 := metrics.XForTime(seq.IntroSlide.Duration)
			w := x2 - x1
			if w < 2 {
				w = 2
			}
			DrawRect(img, x1, y+2, w, TrackLaneHeight-4, ColorIntroSlide)
			DrawRectOutline(img, x1, y+2, w, TrackLaneHeight-4, ColorBorder, 1)
			DrawTextTruncated(img, x1+4, y+TrackLaneHeight/2+4, "INTRO", ColorText, w-8)
			timeOffset = seq.IntroSlide.Duration
		}

		if seq.OutroSlide != nil {
			outroStart := totalDuration - seq.OutroSlide.Duration
			x1 := metrics.XForTime(outroStart)
			x2 := metrics.XForTime(totalDuration)
			w := x2 - x1
			if w < 2 {
				w = 2
			}
			DrawRect(img, x1, y+2, w, TrackLaneHeight-4, ColorOutroSlide)
			DrawRectOutline(img, x1, y+2, w, TrackLaneHeight-4, ColorBorder, 1)
			DrawTextTruncated(img, x1+4, y+TrackLaneHeight/2+4, "OUTRO", ColorText, w-8)
		}

		_ = timeOffset
	}

	return img, nil
}

func drawRuler(img *image.RGBA, metrics *LayoutMetrics) {
	// Ruler background
	DrawRect(img, 0, 0, metrics.CanvasWidth, RulerHeight, ColorRuler)

	// Determine tick interval
	dur := metrics.TotalDuration
	var tickInterval float64
	switch {
	case dur <= 30:
		tickInterval = 5
	case dur <= 120:
		tickInterval = 10
	case dur <= 300:
		tickInterval = 30
	case dur <= 900:
		tickInterval = 60
	default:
		tickInterval = 120
	}

	// Draw ticks
	for t := 0.0; t <= dur; t += tickInterval {
		x := metrics.XForTime(t)
		// Tick line
		DrawRect(img, x, RulerHeight-10, 1, 10, ColorRulerTick)
		// Label
		label := FormatTimecode(t)
		DrawText(img, x+2, RulerHeight-2, label, ColorTextDim)
	}

	// Bottom border
	DrawRect(img, 0, RulerHeight-1, metrics.CanvasWidth, 1, ColorBorder)
}

func drawClipBlock(img *image.RGBA, metrics *LayoutMetrics, clip model.Clip, trackType model.TrackType, assets map[string]*model.Asset, thumbnailDir string, trackY int) {
	x1 := metrics.XForTime(clip.TimelineIn)
	x2 := metrics.XForTime(clip.TimelineOut)
	w := x2 - x1
	if w < 2 {
		w = 2
	}
	y := trackY + 2
	h := TrackLaneHeight - 4

	// Clip color
	clipColor := ColorVideoClip
	if trackType == model.TrackTypeAudio {
		clipColor = ColorAudioClip
	}
	DrawRect(img, x1, y, w, h, clipColor)

	// Try to composite thumbnail for video clips
	if trackType == model.TrackTypeVideo && thumbnailDir != "" && w > 20 {
		thumbPath := filepath.Join(thumbnailDir, clip.ID+".jpg")
		if thumb, err := loadJPEG(thumbPath); err == nil {
			thumbH := h - 4
			thumbW := thumbH * thumb.Bounds().Dx() / thumb.Bounds().Dy()
			if thumbW > w-4 {
				thumbW = w - 4
			}
			scaled := ScaleImage(thumb, thumbW, thumbH)
			draw.Draw(img, image.Rect(x1+2, y+2, x1+2+thumbW, y+2+thumbH), scaled, image.Point{}, draw.Over)
		}
	}

	// Audio waveform decoration
	if trackType == model.TrackTypeAudio && w > 10 {
		mid := y + h/2
		for px := x1 + 2; px < x1+w-2; px += 3 {
			barH := (px*7 + 13) % (h/2 - 4)
			if barH < 3 {
				barH = 3
			}
			DrawRect(img, px, mid-barH/2, 2, barH, ColorText)
		}
	}

	// Clip label
	if w > 30 {
		labelX := x1 + 4
		if trackType == model.TrackTypeVideo && w > 60 {
			labelX = x1 + ThumbnailHeight + 4
		}
		assetName := ""
		if a, ok := assets[clip.AssetID]; ok {
			assetName = a.Name
		}
		dur := clip.TimelineOut - clip.TimelineIn
		label := fmt.Sprintf("%s (%.0fs)", assetName, dur)
		DrawTextTruncated(img, labelX, y+h/2+4, label, ColorText, x1+w-labelX-4)
	}

	// Border
	DrawRectOutline(img, x1, y, w, h, ColorBorder, 1)
}

func renderEmptyTimeline(msg string) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, TimelineWidth, 120))
	DrawRect(img, 0, 0, TimelineWidth, 120, ColorBackground)
	DrawText(img, TimelineWidth/2-len(msg)*4, 65, msg, ColorTextDim)
	return img
}

func computeTotalDuration(seq *model.Sequence) float64 {
	maxEnd := 0.0
	for _, t := range seq.Tracks {
		for _, c := range t.Clips {
			if c.TimelineOut > maxEnd {
				maxEnd = c.TimelineOut
			}
		}
	}
	if seq.IntroSlide != nil {
		maxEnd += seq.IntroSlide.Duration
	}
	if seq.OutroSlide != nil {
		maxEnd += seq.OutroSlide.Duration
	}
	return maxEnd
}

func loadJPEG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := jpeg.Decode(f)
	return img, err
}
