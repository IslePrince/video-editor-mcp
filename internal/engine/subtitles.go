package engine

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"video-editor/internal/model"
)

type SubtitleEntry struct {
	Start float64
	End   float64
	Text  string
}

// ParseVTT parses a WebVTT file into subtitle entries.
func ParseVTT(path string) ([]SubtitleEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read VTT: %w", err)
	}

	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	blocks := strings.Split(strings.TrimSpace(content), "\n\n")
	// Match both HH:MM:SS.mmm and MM:SS.mmm timestamp formats
	tcRe := regexp.MustCompile(`(?:(\d+):)?(\d+):(\d+)\.(\d+)\s*-->\s*(?:(\d+):)?(\d+):(\d+)\.(\d+)`)

	var entries []SubtitleEntry
	for _, block := range blocks {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		for i, line := range lines {
			matches := tcRe.FindStringSubmatch(line)
			if len(matches) < 9 {
				continue
			}
			// Groups: [1]=h_start (optional), [2]=m_start, [3]=s_start, [4]=ms_start,
			//         [5]=h_end (optional), [6]=m_end, [7]=s_end, [8]=ms_end
			startS := parseTCOptionalHour(matches[1], matches[2], matches[3], matches[4])
			endS := parseTCOptionalHour(matches[5], matches[6], matches[7], matches[8])
			text := ""
			if i+1 < len(lines) {
				text = strings.Join(lines[i+1:], "\n")
			}
			entries = append(entries, SubtitleEntry{Start: startS, End: endS, Text: text})
			break
		}
	}
	return entries, nil
}

// RemapSubtitles takes the original VTT entries and a list of clip mappings,
// and produces a new set of entries with timestamps adjusted for the output timeline.
// Each clip mapping defines: source_in, source_out, timeline_offset (where it appears in the output).
type ClipMapping struct {
	SourceIn       float64
	SourceOut      float64
	TimelineOffset float64
}

func RemapSubtitles(entries []SubtitleEntry, mappings []ClipMapping) []SubtitleEntry {
	var remapped []SubtitleEntry

	for _, m := range mappings {
		for _, e := range entries {
			// Check if subtitle overlaps with this clip's source range
			if e.End <= m.SourceIn || e.Start >= m.SourceOut {
				continue
			}

			// Clamp to clip boundaries
			subStart := e.Start
			subEnd := e.End
			if subStart < m.SourceIn {
				subStart = m.SourceIn
			}
			if subEnd > m.SourceOut {
				subEnd = m.SourceOut
			}

			// Remap to timeline position
			newStart := (subStart - m.SourceIn) + m.TimelineOffset
			newEnd := (subEnd - m.SourceIn) + m.TimelineOffset

			remapped = append(remapped, SubtitleEntry{
				Start: newStart,
				End:   newEnd,
				Text:  e.Text,
			})
		}
	}

	return remapped
}

// WriteVTT writes subtitle entries to a VTT file.
func WriteVTT(path string, entries []SubtitleEntry) error {
	var sb strings.Builder
	sb.WriteString("WEBVTT\n\n")

	for i, e := range entries {
		sb.WriteString(fmt.Sprintf("%d\n", i+1))
		sb.WriteString(fmt.Sprintf("%s --> %s\n", formatVTTTime(e.Start), formatVTTTime(e.End)))
		sb.WriteString(e.Text)
		sb.WriteString("\n\n")
	}

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// BuildClipMappings computes timeline offsets for clips based on the sequence structure,
// accounting for intro slide duration and transition overlaps.
func BuildClipMappings(seq *model.Sequence, videoClips []model.Clip, transitionDur float64) []ClipMapping {
	var mappings []ClipMapping

	// Start offset after intro slide
	offset := 0.0
	if seq.IntroSlide != nil {
		offset = seq.IntroSlide.Duration
	}

	for i, clip := range videoClips {
		if i > 0 || seq.IntroSlide != nil {
			// Account for transition overlap
			offset -= transitionDur
			if offset < 0 {
				offset = 0
			}
		}

		mappings = append(mappings, ClipMapping{
			SourceIn:       clip.SourceIn,
			SourceOut:      clip.SourceOut,
			TimelineOffset: offset,
		})

		clipDur := clip.SourceOut - clip.SourceIn
		offset += clipDur
	}

	return mappings
}

func parseTCOptionalHour(h, m, s, ms string) float64 {
	hi := 0
	if h != "" {
		hi, _ = strconv.Atoi(h)
	}
	mi, _ := strconv.Atoi(m)
	si, _ := strconv.Atoi(s)
	msi, _ := strconv.Atoi(ms)
	return float64(hi)*3600 + float64(mi)*60 + float64(si) + float64(msi)/1000
}

func parseTC(h, m, s, ms string) float64 {
	hi, _ := strconv.Atoi(h)
	mi, _ := strconv.Atoi(m)
	si, _ := strconv.Atoi(s)
	msi, _ := strconv.Atoi(ms)
	return float64(hi)*3600 + float64(mi)*60 + float64(si) + float64(msi)/1000
}

func formatVTTTime(seconds float64) string {
	h := int(seconds) / 3600
	m := (int(seconds) % 3600) / 60
	s := int(seconds) % 60
	ms := int((seconds - float64(int(seconds))) * 1000)
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}
