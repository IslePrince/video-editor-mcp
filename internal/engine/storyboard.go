package engine

import (
	"fmt"
	"image"
	"image/draw"
	"sort"

	"video-editor/internal/model"
)

type storyEntry struct {
	Label     string
	Duration  float64
	ClipID    string
	IsSlide   bool
	SlideText string
	SrcRange  string
}

// RenderStoryboardImage generates a filmstrip-style storyboard.
func RenderStoryboardImage(seq *model.Sequence, assets map[string]*model.Asset, thumbnailDir string) (*image.RGBA, error) {
	var entries []storyEntry

	// Intro slide
	if seq.IntroSlide != nil {
		entries = append(entries, storyEntry{
			Label:     "INTRO",
			Duration:  seq.IntroSlide.Duration,
			IsSlide:   true,
			SlideText: seq.IntroSlide.Text,
		})
	}

	// Collect video clips
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

	for _, clip := range clips {
		assetName := clip.AssetID[:8]
		if a, ok := assets[clip.AssetID]; ok {
			assetName = a.Name
			if len(assetName) > 20 {
				assetName = assetName[:18] + ".."
			}
		}
		dur := clip.TimelineOut - clip.TimelineIn
		// Show source timecode range
		srcLabel := fmt.Sprintf("%s-%s",
			FormatTimecode(clip.SourceIn),
			FormatTimecode(clip.SourceOut),
		)
		entries = append(entries, storyEntry{
			Label:    assetName,
			Duration: dur,
			ClipID:   clip.ID,
			SrcRange: srcLabel,
		})
	}

	// Outro slide
	if seq.OutroSlide != nil {
		entries = append(entries, storyEntry{
			Label:     "OUTRO",
			Duration:  seq.OutroSlide.Duration,
			IsSlide:   true,
			SlideText: seq.OutroSlide.Text,
		})
	}

	if len(entries) == 0 {
		return renderEmptyStoryboard(), nil
	}

	// Compute canvas size
	frameW := StoryThumbWidth
	frameH := StoryThumbHeight
	entryW := frameW + StoryArrowWidth
	totalW := len(entries)*entryW - StoryArrowWidth + StoryPadding*2
	if totalW > 2400 {
		// Scale down frame width
		frameW = (2400 - StoryPadding*2 + StoryArrowWidth) / len(entries) - StoryArrowWidth
		if frameW < 80 {
			frameW = 80
		}
		entryW = frameW + StoryArrowWidth
		totalW = len(entries)*entryW - StoryArrowWidth + StoryPadding*2
	}
	totalH := StoryPadding + frameH + StoryLabelHeight + StoryPadding

	img := image.NewRGBA(image.Rect(0, 0, totalW, totalH))
	DrawRect(img, 0, 0, totalW, totalH, ColorStoryBg)

	// Draw each entry
	for i, entry := range entries {
		x := StoryPadding + i*entryW
		y := StoryPadding

		// Frame border
		DrawRect(img, x, y, frameW, frameH, ColorStoryFrame)

		if entry.IsSlide {
			// Slide: draw colored background with text
			slideColor := ColorIntroSlide
			if entry.Label == "OUTRO" {
				slideColor = ColorOutroSlide
			}
			DrawRect(img, x+1, y+1, frameW-2, frameH-2, slideColor)
			// Center the label
			DrawTextTruncated(img, x+4, y+frameH/2+4, entry.Label, ColorText, frameW-8)
		} else {
			// Clip: try to load thumbnail
			thumbLoaded := false
			if thumbnailDir != "" {
				thumbPath := thumbnailDir + "/" + entry.ClipID + ".jpg"
				if thumb, err := loadJPEG(thumbPath); err == nil {
					// Scale to fit frame
					scaled := ScaleImage(thumb, frameW-2, frameH-2)
					draw.Draw(img, image.Rect(x+1, y+1, x+frameW-1, y+frameH-1), scaled, image.Point{}, draw.Over)
					thumbLoaded = true
				}
			}
			if !thumbLoaded {
				DrawRect(img, x+1, y+1, frameW-2, frameH-2, ColorVideoClip)
			}
		}

		// Frame outline
		DrawRectOutline(img, x, y, frameW, frameH, ColorBorder, 1)

		// Labels below frame
		labelY := y + frameH + 13
		DrawTextTruncated(img, x+2, labelY, entry.Label, ColorText, frameW-4)
		// Duration + source range
		durLabel := fmt.Sprintf("%.0fs", entry.Duration)
		if entry.SrcRange != "" {
			durLabel += " " + entry.SrcRange
		}
		DrawTextTruncated(img, x+2, labelY+14, durLabel, ColorTextDim, frameW-4)

		// Arrow to next
		if i < len(entries)-1 {
			arrowX := x + frameW + 4
			arrowY := y + frameH/2
			// Simple arrow: line + head
			for ax := 0; ax < StoryArrowWidth-8; ax++ {
				DrawRect(img, arrowX+ax, arrowY, 1, 1, ColorArrow)
			}
			// Arrow head
			headX := arrowX + StoryArrowWidth - 10
			for dy := -4; dy <= 4; dy++ {
				lineLen := 4 - abs(dy)
				for dx := 0; dx <= lineLen; dx++ {
					DrawRect(img, headX+dx, arrowY+dy, 1, 1, ColorArrow)
				}
			}
		}
	}

	return img, nil
}

func renderEmptyStoryboard() *image.RGBA {
	w := 400
	h := StoryThumbHeight + StoryLabelHeight + StoryPadding*2
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	DrawRect(img, 0, 0, w, h, ColorStoryBg)
	DrawText(img, w/2-60, h/2+4, "No clips", ColorTextDim)
	return img
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
