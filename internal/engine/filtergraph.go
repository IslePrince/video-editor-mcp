package engine

import (
	"fmt"
	"sort"
	"strings"

	"video-editor/internal/model"
)

type InputFile struct {
	Index int
	Path  string
}

type FilterGraph struct {
	Inputs      []InputFile
	FilterChain string
	VideoOut    string
	AudioOut    string
}

// BuildFilterGraph creates an FFmpeg filter graph from a sequence.
// It handles: intro/outro slides (with optional logo), clip trimming, crop, dynamic transitions,
// image overlays, text overlays, and subtitle burn-in with custom styling.
func BuildFilterGraph(seq *model.Sequence, assets map[string]*model.Asset, settings model.ProjectSettings, subtitlePath string) (*FilterGraph, error) {
	fg := &FilterGraph{}
	var filters []string
	inputIdx := 0

	w := settings.Width
	h := settings.Height
	fps := settings.FrameRate.Num
	if settings.FrameRate.Den > 1 {
		fps = settings.FrameRate.Num / settings.FrameRate.Den
	}
	if fps <= 0 {
		fps = 30
	}

	// Collect all video clips sorted by timeline_in
	var videoClips []model.Clip
	for _, track := range seq.Tracks {
		if track.Type != model.TrackTypeVideo || track.Muted {
			continue
		}
		for _, clip := range track.Clips {
			if clip.Enabled {
				videoClips = append(videoClips, clip)
			}
		}
	}
	sort.Slice(videoClips, func(i, j int) bool {
		return videoClips[i].TimelineIn < videoClips[j].TimelineIn
	})

	if len(videoClips) == 0 && seq.IntroSlide == nil && seq.OutroSlide == nil {
		return nil, fmt.Errorf("sequence has no clips or slides")
	}

	// Build list of video segments (slides + clips)
	type segment struct {
		label        string
		alabel       string
		duration     float64
		transitionIn *model.Transition // transition into this segment
	}
	var segments []segment

	// --- Map asset inputs first (so we know indices) ---
	assetInputMap := map[string]int{}
	for _, clip := range videoClips {
		if _, exists := assetInputMap[clip.AssetID]; !exists {
			asset, ok := assets[clip.AssetID]
			if !ok {
				return nil, fmt.Errorf("asset not found for clip: %s", clip.AssetID)
			}
			assetInputMap[clip.AssetID] = inputIdx
			fg.Inputs = append(fg.Inputs, InputFile{Index: inputIdx, Path: asset.FilePath})
			inputIdx++
		}
	}

	// Also map overlay image assets
	overlayInputMap := map[string]int{} // overlayID -> inputIdx
	for _, ov := range seq.Overlays {
		if _, exists := assetInputMap[ov.AssetID]; !exists {
			asset, ok := assets[ov.AssetID]
			if !ok {
				continue
			}
			assetInputMap[ov.AssetID] = inputIdx
			fg.Inputs = append(fg.Inputs, InputFile{Index: inputIdx, Path: asset.FilePath})
			overlayInputMap[ov.ID] = inputIdx
			inputIdx++
		} else {
			overlayInputMap[ov.ID] = assetInputMap[ov.AssetID]
		}
	}

	// Map logo assets for intro/outro slides
	slideLogoInputMap := map[string]int{} // "intro" or "outro" -> inputIdx
	for _, label := range []string{"intro", "outro"} {
		var slide *model.Slide
		if label == "intro" {
			slide = seq.IntroSlide
		} else {
			slide = seq.OutroSlide
		}
		if slide == nil || slide.LogoAssetID == "" {
			continue
		}
		if _, exists := assetInputMap[slide.LogoAssetID]; !exists {
			asset, ok := assets[slide.LogoAssetID]
			if !ok {
				continue
			}
			assetInputMap[slide.LogoAssetID] = inputIdx
			fg.Inputs = append(fg.Inputs, InputFile{Index: inputIdx, Path: asset.FilePath})
			slideLogoInputMap[label] = inputIdx
			inputIdx++
		} else {
			slideLogoInputMap[label] = assetInputMap[slide.LogoAssetID]
		}
	}

	// --- Intro slide ---
	if seq.IntroSlide != nil {
		vLabel, aLabel := buildSlideFilters(&filters, seq.IntroSlide, "intro", w, h, fps, slideLogoInputMap)
		segments = append(segments, segment{label: vLabel, alabel: aLabel, duration: seq.IntroSlide.Duration})
	}

	// --- Source clips ---
	for ci, clip := range videoClips {
		idx := assetInputMap[clip.AssetID]
		clipDur := clip.SourceOut - clip.SourceIn
		vLabel := fmt.Sprintf("[clip%d_v]", ci)
		aLabel := fmt.Sprintf("[clip%d_a]", ci)

		// Video chain: trim → setpts → fps → [crop] → scale+pad → format → settb
		videoChain := fmt.Sprintf("[%d:v]trim=start=%g:end=%g,setpts=PTS-STARTPTS,fps=%d",
			idx, clip.SourceIn, clip.SourceOut, fps)

		// Crop filter (before scale) — per-clip crop takes priority, then sequence-level crop_mode
		cropSettings := clip.Crop
		if cropSettings == nil && seq.CropMode != "" && seq.CropMode != "fit" {
			cropSettings = &model.CropSettings{Mode: seq.CropMode}
		}
		if cropSettings != nil {
			videoChain += buildCropFilter(cropSettings, w, h)
			// After cropping, scale to fill frame (no letterboxing)
			videoChain += fmt.Sprintf(",scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,format=yuv420p,settb=AVTB%s",
				w, h, w, h, vLabel)
		} else {
			// No crop — scale to fit with letterboxing
			videoChain += fmt.Sprintf(",scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,format=yuv420p,settb=AVTB%s",
				w, h, w, h, vLabel)
		}
		filters = append(filters, videoChain)

		// Audio: atrim, asetpts
		filters = append(filters, fmt.Sprintf(
			"[%d:a]atrim=start=%g:end=%g,asetpts=PTS-STARTPTS%s",
			idx, clip.SourceIn, clip.SourceOut, aLabel,
		))

		seg := segment{label: vLabel, alabel: aLabel, duration: clipDur}
		if clip.TransitionIn != nil {
			seg.transitionIn = clip.TransitionIn
		}
		segments = append(segments, seg)
	}

	// --- Outro slide ---
	if seq.OutroSlide != nil {
		vLabel, aLabel := buildSlideFilters(&filters, seq.OutroSlide, "outro", w, h, fps, slideLogoInputMap)
		segments = append(segments, segment{label: vLabel, alabel: aLabel, duration: seq.OutroSlide.Duration})
	}

	if len(segments) == 0 {
		return nil, fmt.Errorf("no segments to render")
	}

	// --- Combine segments with transitions ---
	if len(segments) == 1 {
		fg.VideoOut = segments[0].label
		fg.AudioOut = segments[0].alabel
	} else {
		currentV := segments[0].label
		currentA := segments[0].alabel
		offset := segments[0].duration

		for i := 1; i < len(segments); i++ {
			// Determine transition type and duration
			transType := "fade"
			transDur := 1.0

			if segments[i].transitionIn != nil {
				t := segments[i].transitionIn
				if t.Duration > 0 {
					transDur = t.Duration
				}
				transType = mapTransitionType(t.Type, t.Params)
			}

			// Handle "none" transition — just concatenate
			if transType == "none" {
				// No crossfade, segments play back-to-back
				nextVLabel := fmt.Sprintf("[xv%d]", i)
				nextALabel := fmt.Sprintf("[xa%d]", i)
				filters = append(filters, fmt.Sprintf(
					"%s%sconcat=n=2:v=1:a=0%s",
					currentV, segments[i].label, nextVLabel,
				))
				filters = append(filters, fmt.Sprintf(
					"%s%sconcat=n=2:v=0:a=1%s",
					currentA, segments[i].alabel, nextALabel,
				))
				currentV = nextVLabel
				currentA = nextALabel
				offset += segments[i].duration
				continue
			}

			// Clamp transition duration
			maxDur := segments[i-1].duration
			if segments[i].duration < maxDur {
				maxDur = segments[i].duration
			}
			if transDur > maxDur*0.5 {
				transDur = maxDur * 0.5
			}
			if transDur < 0.1 {
				transDur = 0.1
			}

			xfadeOffset := offset - transDur
			if xfadeOffset < 0 {
				xfadeOffset = 0
			}
			nextVLabel := fmt.Sprintf("[xv%d]", i)
			nextALabel := fmt.Sprintf("[xa%d]", i)

			filters = append(filters, fmt.Sprintf(
				"%s%sxfade=transition=%s:duration=%g:offset=%g%s",
				currentV, segments[i].label, transType, transDur, xfadeOffset, nextVLabel,
			))
			filters = append(filters, fmt.Sprintf(
				"%s%sacrossfade=d=%g%s",
				currentA, segments[i].alabel, transDur, nextALabel,
			))

			currentV = nextVLabel
			currentA = nextALabel
			offset = xfadeOffset + segments[i].duration
		}
		fg.VideoOut = currentV
		fg.AudioOut = currentA
	}

	// --- Image overlays ---
	for i, ov := range seq.Overlays {
		ovIdx, ok := overlayInputMap[ov.ID]
		if !ok {
			continue
		}

		// Scale overlay image
		scaleStr := ""
		if ov.Width != "" {
			if strings.HasSuffix(ov.Width, "%") {
				// Percentage of frame width
				scaleStr = fmt.Sprintf("scale=iw*%s/%s:-1", strings.TrimSuffix(ov.Width, "%"), "100")
			} else {
				scaleStr = fmt.Sprintf("scale=%s:-1", ov.Width)
			}
		}

		ovScaledLabel := fmt.Sprintf("[ov%d_s]", i)
		scaleChain := fmt.Sprintf("[%d:v]", ovIdx)
		if scaleStr != "" {
			scaleChain += scaleStr + ","
		}
		scaleChain += "format=rgba"

		// Apply opacity
		if ov.Opacity > 0 && ov.Opacity < 1.0 {
			scaleChain += fmt.Sprintf(",colorchannelmixer=aa=%g", ov.Opacity)
		}
		scaleChain += ovScaledLabel
		filters = append(filters, scaleChain)

		// Compute position
		x, y := computeOverlayPosition(ov.Position, ov.X, ov.Y, ov.Padding)

		// Build overlay filter
		ovOutLabel := fmt.Sprintf("[ov%d_out]", i)
		overlayStr := fmt.Sprintf("%s%soverlay=%s:%s", fg.VideoOut, ovScaledLabel, x, y)

		// Time-limited display
		if ov.StartTime > 0 || ov.EndTime > 0 {
			enableExpr := buildEnableExpr(ov.StartTime, ov.EndTime)
			overlayStr += ":enable='" + enableExpr + "'"
		}
		overlayStr += ovOutLabel
		filters = append(filters, overlayStr)
		fg.VideoOut = ovOutLabel
	}

	// --- Text overlays ---
	for i, to := range seq.TextOverlays {
		toOutLabel := fmt.Sprintf("[to%d_out]", i)
		dtFilter := buildDrawtextFilter(&to)

		// Time-limited display
		if to.StartTime > 0 || to.EndTime > 0 {
			enableExpr := buildEnableExpr(to.StartTime, to.EndTime)
			dtFilter += ":enable='" + enableExpr + "'"
		}

		filters = append(filters, fmt.Sprintf("%s%s%s", fg.VideoOut, dtFilter, toOutLabel))
		fg.VideoOut = toOutLabel
	}

	// --- Subtitle burn-in ---
	if subtitlePath != "" {
		subLabel := "[subbed]"
		escapedPath := strings.ReplaceAll(subtitlePath, "\\", "\\\\")
		escapedPath = strings.ReplaceAll(escapedPath, ":", "\\:")

		subFilter := fmt.Sprintf("%ssubtitles='%s'", fg.VideoOut, escapedPath)

		// Apply subtitle style if set
		if seq.SubtitleStyle != nil {
			forceStyle := buildSubtitleForceStyle(seq.SubtitleStyle)
			if forceStyle != "" {
				subFilter += ":force_style='" + forceStyle + "'"
			}
		}
		subFilter += subLabel
		filters = append(filters, subFilter)
		fg.VideoOut = subLabel
	}

	fg.FilterChain = strings.Join(filters, ";\n")
	return fg, nil
}

// buildSlideFilters generates filter chain for an intro/outro slide with optional logo overlay.
func buildSlideFilters(filters *[]string, slide *model.Slide, label string, w, h, fps int, logoInputMap map[string]int) (string, string) {
	fontSize := slide.FontSize
	if fontSize == 0 {
		fontSize = 48
	}
	fontColor := slide.FontColor
	if fontColor == "" {
		fontColor = "white"
	}
	bgColor := slide.BgColor
	if bgColor == "" {
		bgColor = "black"
	}
	escapedText := escapeDrawtext(slide.Text)

	vLabel := fmt.Sprintf("[%s_v]", label)
	aLabel := fmt.Sprintf("[%s_a]", label)

	// Build drawtext with optional font family
	dtParams := fmt.Sprintf("text='%s':fontsize=%d:fontcolor=%s:x=(w-text_w)/2:y=(h-text_h)/2",
		escapedText, fontSize, fontColor)
	if slide.FontFamily != "" {
		dtParams += fmt.Sprintf(":font='%s'", slide.FontFamily)
	}

	baseVLabel := vLabel
	if _, hasLogo := logoInputMap[label]; hasLogo {
		baseVLabel = fmt.Sprintf("[%s_base]", label)
	}

	*filters = append(*filters, fmt.Sprintf(
		"color=c=%s:s=%dx%d:d=%g:r=%d,format=yuv420p,settb=AVTB,drawtext=%s%s",
		bgColor, w, h, slide.Duration, fps, dtParams, baseVLabel,
	))
	*filters = append(*filters, fmt.Sprintf(
		"anullsrc=r=48000:cl=stereo,atrim=0:%g,asetpts=PTS-STARTPTS%s",
		slide.Duration, aLabel,
	))

	// Logo overlay on slide
	if logoIdx, hasLogo := logoInputMap[label]; hasLogo {
		logoScaled := fmt.Sprintf("[%s_logo]", label)
		*filters = append(*filters, fmt.Sprintf(
			"[%d:v]scale=-1:%.0f,format=rgba%s",
			logoIdx, float64(h)*0.15, logoScaled,
		))
		*filters = append(*filters, fmt.Sprintf(
			"%s%soverlay=(W-w)/2:H*3/4-h/2%s",
			baseVLabel, logoScaled, vLabel,
		))
	}

	return vLabel, aLabel
}

func buildCropFilter(crop *model.CropSettings, projW, projH int) string {
	if crop.Mode != "" {
		switch crop.Mode {
		case "center_crop":
			// Crop to fill the project aspect ratio from center
			return fmt.Sprintf(",crop=if(gt(iw/ih\\,%d/%d)\\,ih*%d/%d\\,iw):if(gt(iw/ih\\,%d/%d)\\,ih\\,iw*%d/%d):(iw-ow)/2:(ih-oh)/2",
				projW, projH, projW, projH, projW, projH, projH, projW)
		case "speaker_focus_left":
			return ",crop=iw/2:ih:0:0"
		case "speaker_focus_right":
			return ",crop=iw/2:ih:iw/2:0"
		}
	}
	if crop.CropWidth > 0 && crop.CropHeight > 0 {
		return fmt.Sprintf(",crop=%d:%d:%d:%d", crop.CropWidth, crop.CropHeight, crop.CropX, crop.CropY)
	}
	return ""
}

func mapTransitionType(t model.TransitionType, params map[string]string) string {
	switch t {
	case model.TransitionFade, model.TransitionCrossfade, model.TransitionDissolve:
		return "fade"
	case model.TransitionWipe:
		dir := params["direction"]
		switch dir {
		case "right":
			return "wiperight"
		case "up":
			return "wipeup"
		case "down":
			return "wipedown"
		default:
			return "wipeleft"
		}
	case model.TransitionSlide:
		dir := params["direction"]
		switch dir {
		case "right":
			return "slideright"
		case "up":
			return "slideup"
		case "down":
			return "slidedown"
		default:
			return "slideleft"
		}
	case model.TransitionDipToColor:
		color := params["color"]
		if color == "white" {
			return "fadewhite"
		}
		return "fadeblack"
	case model.TransitionNone:
		return "none"
	default:
		return "fade"
	}
}

func computeOverlayPosition(position string, customX, customY, padding int) (string, string) {
	if padding == 0 {
		padding = 20
	}
	p := fmt.Sprintf("%d", padding)

	switch position {
	case "top_left":
		return p, p
	case "top_right":
		return fmt.Sprintf("W-w-%s", p), p
	case "bottom_left":
		return p, fmt.Sprintf("H-h-%s", p)
	case "bottom_right":
		return fmt.Sprintf("W-w-%s", p), fmt.Sprintf("H-h-%s", p)
	case "center":
		return "(W-w)/2", "(H-h)/2"
	case "custom":
		return fmt.Sprintf("%d", customX), fmt.Sprintf("%d", customY)
	default:
		return fmt.Sprintf("W-w-%s", p), p // default top_right
	}
}

func buildEnableExpr(startTime, endTime float64) string {
	if startTime > 0 && endTime > 0 {
		return fmt.Sprintf("between(t\\,%g\\,%g)", startTime, endTime)
	}
	if startTime > 0 {
		return fmt.Sprintf("gte(t\\,%g)", startTime)
	}
	if endTime > 0 {
		return fmt.Sprintf("lte(t\\,%g)", endTime)
	}
	return "1"
}

func buildDrawtextFilter(to *model.TextOverlay) string {
	escapedText := escapeDrawtext(to.Text)

	fontSize := to.FontSize
	if fontSize == 0 {
		fontSize = 36
	}
	fontColor := to.FontColor
	if fontColor == "" {
		fontColor = "white"
	}
	fontFamily := to.FontFamily
	if fontFamily == "" {
		fontFamily = "DM Sans"
	}

	// Compute position
	x, y := computeTextPosition(to.Position, to.X, to.Y, to.Padding)

	dt := fmt.Sprintf("drawtext=text='%s':fontsize=%d:fontcolor=%s:font='%s':x=%s:y=%s",
		escapedText, fontSize, fontColor, fontFamily, x, y)

	if to.Bold {
		dt = strings.Replace(dt, fmt.Sprintf(":font='%s'", fontFamily), fmt.Sprintf(":font='%s Bold'", fontFamily), 1)
	}

	// Background box
	if to.BgColor != "" {
		bgOpacity := to.BgOpacity
		if bgOpacity == 0 {
			bgOpacity = 0.8
		}
		padding := to.Padding
		if padding == 0 {
			padding = 10
		}
		dt += fmt.Sprintf(":box=1:boxcolor=%s@%g:boxborderw=%d",
			to.BgColor, bgOpacity, padding)
	}

	return dt
}

func computeTextPosition(position string, customX, customY, padding int) (string, string) {
	if padding == 0 {
		padding = 10
	}

	switch position {
	case "top":
		return "(w-text_w)/2", fmt.Sprintf("%d", padding)
	case "center":
		return "(w-text_w)/2", "(h-text_h)/2"
	case "bottom":
		return "(w-text_w)/2", fmt.Sprintf("h-text_h-%d", padding+40)
	case "custom":
		return fmt.Sprintf("%d", customX), fmt.Sprintf("%d", customY)
	default:
		return "(w-text_w)/2", fmt.Sprintf("h-text_h-%d", padding+40) // default bottom
	}
}

func buildSubtitleForceStyle(style *model.SubtitleStyle) string {
	var parts []string

	if style.FontFamily != "" {
		parts = append(parts, fmt.Sprintf("FontName=%s", style.FontFamily))
	}
	if style.FontSize > 0 {
		parts = append(parts, fmt.Sprintf("FontSize=%d", style.FontSize))
	}
	if style.FontColor != "" {
		parts = append(parts, fmt.Sprintf("PrimaryColour=%s", hexToASS(style.FontColor)))
	}
	if style.OutlineColor != "" {
		parts = append(parts, fmt.Sprintf("OutlineColour=%s", hexToASS(style.OutlineColor)))
	}
	if style.OutlineWidth > 0 {
		parts = append(parts, fmt.Sprintf("Outline=%d", style.OutlineWidth))
	}
	if style.BgColor != "" {
		bgASS := hexToASS(style.BgColor)
		if style.BgOpacity > 0 && style.BgOpacity < 1.0 {
			// Modify alpha channel
			alpha := int((1.0 - style.BgOpacity) * 255)
			bgASS = fmt.Sprintf("&H%02X%s", alpha, bgASS[3:])
		}
		parts = append(parts, fmt.Sprintf("BackColour=%s", bgASS))
		parts = append(parts, "BorderStyle=4") // opaque box behind text
	}
	if style.MarginBottom > 0 {
		parts = append(parts, fmt.Sprintf("MarginV=%d", style.MarginBottom))
	}
	if style.Position != "" {
		switch style.Position {
		case "top":
			parts = append(parts, "Alignment=8") // top-center
		case "center":
			parts = append(parts, "Alignment=5") // mid-center
		case "bottom":
			parts = append(parts, "Alignment=2") // bottom-center
		}
	}

	return strings.Join(parts, ",")
}

// hexToASS converts a hex color like "#FFFFFF" to ASS format "&H00FFFFFF" (AABBGGRR).
func hexToASS(hex string) string {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) == 6 {
		r := hex[0:2]
		g := hex[2:4]
		b := hex[4:6]
		return "&H00" + b + g + r
	}
	return "&H00FFFFFF"
}

func escapeDrawtext(text string) string {
	text = strings.ReplaceAll(text, "\\", "\\\\")
	text = strings.ReplaceAll(text, "'", "\\'")
	text = strings.ReplaceAll(text, ":", "\\:")
	text = strings.ReplaceAll(text, "\n", "\\n")
	return text
}
