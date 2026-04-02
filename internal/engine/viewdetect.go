package engine

import (
	"path/filepath"
	"regexp"
	"strings"
)

// DetectViewType identifies the Zoom recording view type from a filename.
// Returns: "speaker", "gallery", "active_video", "combined", or ""
func DetectViewType(filename string) string {
	base := strings.ToLower(filepath.Base(filename))

	if strings.Contains(base, "_as_") {
		return "speaker"
	}
	if strings.Contains(base, "_gvo_") {
		return "gallery"
	}
	if strings.Contains(base, "_avo_") {
		return "active_video"
	}

	// If it matches the Zoom recording pattern but has no view suffix, it's the combined view
	zoomPattern := regexp.MustCompile(`^gmt\d{8}-\d{6}_recording.*_\d+x\d+\.mp4$`)
	if zoomPattern.MatchString(base) {
		return "combined"
	}

	return ""
}

// ExtractRecordingGroup extracts a shared recording identifier from a Zoom filename.
// All views of the same recording share the same group key.
// e.g. "GMT20260313-182842_Recording_as_2806x1320.mp4" → "GMT20260313-182842_Recording"
func ExtractRecordingGroup(filename string) string {
	base := filepath.Base(filename)

	// Remove extension
	name := strings.TrimSuffix(base, filepath.Ext(base))

	// Strip known view suffixes and resolution
	// Pattern: _as_WxH, _gvo_WxH, _avo_WxH, or _WxH at the end
	stripRe := regexp.MustCompile(`(_(?:as|gvo|avo))?_\d+x\d+$`)
	group := stripRe.ReplaceAllString(name, "")

	if group == name {
		// Didn't match the pattern — not a Zoom recording
		return ""
	}

	return group
}
