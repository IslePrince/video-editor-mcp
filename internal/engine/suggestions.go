package engine

import (
	"sort"
	"strings"
)

// ClipSuggestion represents a suggested clip from transcript analysis.
type ClipSuggestion struct {
	Title             string  `json:"title"`
	StartTime         float64 `json:"start_time"`
	EndTime           float64 `json:"end_time"`
	TranscriptPreview string  `json:"transcript_preview"`
	Reason            string  `json:"reason"`
	Score             float64 `json:"score"`
}

type clipSegment struct {
	startIdx int
	endIdx   int
	start    float64
	end      float64
	text     string
	score    float64
}

// SuggestClips analyzes transcript entries and returns clip-worthy segments.
func SuggestClips(entries []SubtitleEntry, maxClips int, minDuration, maxDuration float64) []ClipSuggestion {
	if len(entries) == 0 {
		return []ClipSuggestion{}
	}

	// Identify natural break points (gaps > 2 seconds between entries)
	var segments []clipSegment
	curStart := 0

	for i := 1; i <= len(entries); i++ {
		isBreak := i == len(entries)
		if !isBreak {
			gap := entries[i].Start - entries[i-1].End
			if gap > 2.0 {
				isBreak = true
			}
		}

		if isBreak {
			var parts []string
			for j := curStart; j < i; j++ {
				parts = append(parts, entries[j].Text)
			}

			segments = append(segments, clipSegment{
				startIdx: curStart,
				endIdx:   i - 1,
				start:    entries[curStart].Start,
				end:      entries[i-1].End,
				text:     strings.Join(parts, " "),
			})
			if i < len(entries) {
				curStart = i
			}
		}
	}

	// Merge short consecutive segments to reach minimum duration
	var merged []clipSegment
	for i := 0; i < len(segments); {
		cur := segments[i]
		j := i + 1
		for j < len(segments) && cur.end-cur.start < minDuration {
			cur.end = segments[j].end
			cur.endIdx = segments[j].endIdx
			cur.text += " " + segments[j].text
			j++
		}
		dur := cur.end - cur.start
		if dur >= minDuration && dur <= maxDuration {
			merged = append(merged, cur)
		}
		i = j
	}

	// Also try sliding window approach for better coverage
	if len(merged) < maxClips*3 {
		for i := 0; i < len(entries); i += 5 {
			for j := i + 1; j < len(entries); j++ {
				dur := entries[j].End - entries[i].Start
				if dur < minDuration {
					continue
				}
				if dur > maxDuration {
					break
				}
				var parts []string
				for k := i; k <= j; k++ {
					parts = append(parts, entries[k].Text)
				}
				merged = append(merged, clipSegment{
					startIdx: i,
					endIdx:   j,
					start:    entries[i].Start,
					end:      entries[j].End,
					text:     strings.Join(parts, " "),
				})
				break
			}
		}
	}

	// Score each segment
	for i := range merged {
		merged[i].score = scoreSegment(merged[i].text)
	}

	// Apply temporal diversity: divide video into time buckets and pick
	// the best clip from each bucket, then fill remaining slots with
	// the best overall clips not yet selected.
	selected := selectDiverse(merged, maxClips)

	// Sort by time
	sort.Slice(selected, func(a, b int) bool {
		return selected[a].start < selected[b].start
	})

	suggestions := make([]ClipSuggestion, 0, len(selected))
	for _, s := range selected {
		preview := s.text
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}

		suggestions = append(suggestions, ClipSuggestion{
			Title:             suggestTitle(s.text),
			StartTime:         s.start,
			EndTime:           s.end,
			TranscriptPreview: preview,
			Reason:            suggestReason(s.text, s.score),
			Score:             s.score,
		})
	}

	return suggestions
}

// selectDiverse picks clips spread across the video's timeline.
// It divides the total duration into maxClips equal buckets, picks the
// best-scoring clip from each bucket, then fills any remaining slots
// with the best overall non-overlapping clips.
func selectDiverse(candidates []clipSegment, maxClips int) []clipSegment {
	if len(candidates) == 0 {
		return nil
	}

	// Find total time range
	minTime := candidates[0].start
	maxTime := candidates[0].end
	for _, c := range candidates {
		if c.start < minTime {
			minTime = c.start
		}
		if c.end > maxTime {
			maxTime = c.end
		}
	}

	totalDuration := maxTime - minTime
	if totalDuration <= 0 {
		// Fallback to score-based selection
		return selectByScore(candidates, maxClips)
	}

	bucketSize := totalDuration / float64(maxClips)
	if bucketSize < 30 {
		// Video too short for meaningful buckets, use score-based
		return selectByScore(candidates, maxClips)
	}

	// Sort candidates by score descending
	sort.Slice(candidates, func(a, b int) bool {
		return candidates[a].score > candidates[b].score
	})

	// Assign each candidate to a bucket based on its midpoint
	type bucketEntry struct {
		seg   clipSegment
		score float64
	}
	buckets := make([][]bucketEntry, maxClips)
	for _, c := range candidates {
		midpoint := (c.start + c.end) / 2
		bucketIdx := int((midpoint - minTime) / bucketSize)
		if bucketIdx >= maxClips {
			bucketIdx = maxClips - 1
		}
		if bucketIdx < 0 {
			bucketIdx = 0
		}
		buckets[bucketIdx] = append(buckets[bucketIdx], bucketEntry{seg: c, score: c.score})
	}

	// Pick the best clip from each bucket
	var selected []clipSegment
	usedBuckets := make(map[int]bool)
	for i, bucket := range buckets {
		if len(bucket) == 0 {
			continue
		}
		// Already sorted by score, first is best
		selected = append(selected, bucket[0].seg)
		usedBuckets[i] = true
	}

	// If we have fewer clips than maxClips, fill from remaining candidates
	if len(selected) < maxClips {
		for _, c := range candidates {
			if len(selected) >= maxClips {
				break
			}
			// Check it doesn't overlap with already selected
			overlaps := false
			for _, s := range selected {
				if c.start < s.end && c.end > s.start {
					overlaps = true
					break
				}
			}
			if !overlaps {
				selected = append(selected, c)
			}
		}
	}

	// Deduplicate overlapping segments (shouldn't happen but safety check)
	var deduped []clipSegment
	for _, s := range selected {
		overlaps := false
		for _, d := range deduped {
			if s.start < d.end && s.end > d.start {
				overlaps = true
				break
			}
		}
		if !overlaps {
			deduped = append(deduped, s)
		}
	}

	if len(deduped) > maxClips {
		deduped = deduped[:maxClips]
	}

	return deduped
}

// selectByScore is the fallback when temporal diversity isn't meaningful.
func selectByScore(candidates []clipSegment, maxClips int) []clipSegment {
	sort.Slice(candidates, func(a, b int) bool {
		return candidates[a].score > candidates[b].score
	})

	var deduped []clipSegment
	for _, s := range candidates {
		overlaps := false
		for _, d := range deduped {
			if s.start < d.end && s.end > d.start {
				overlaps = true
				break
			}
		}
		if !overlaps {
			deduped = append(deduped, s)
		}
		if len(deduped) >= maxClips {
			break
		}
	}

	return deduped
}

func scoreSegment(text string) float64 {
	score := 0.0
	lower := strings.ToLower(text)

	score += float64(strings.Count(text, "!")) * 2.0
	score += float64(strings.Count(text, "?")) * 1.5

	strongWords := []string{
		"absolutely", "definitely", "incredible", "amazing", "important",
		"critical", "game changer", "breakthrough", "exactly", "perfect",
		"key", "secret", "biggest", "easiest", "hardest", "never", "always",
		"love", "hate", "believe", "think about", "imagine",
	}
	for _, w := range strongWords {
		if strings.Contains(lower, w) {
			score += 3.0
		}
	}

	// Content-based scoring: actionable advice keywords
	adviceWords := []string{
		"tip", "strategy", "my advice", "what works", "the key is",
		"here's what", "the secret is", "i recommend", "the trick",
		"lesson learned", "biggest mistake", "best thing",
	}
	for _, w := range adviceWords {
		if strings.Contains(lower, w) {
			score += 3.0
		}
	}

	// Speaker change indicators (back-and-forth is engaging)
	speakerChanges := 0
	for _, sep := range []string{"\n", ": "} {
		speakerChanges += strings.Count(text, sep)
	}
	if speakerChanges >= 2 && speakerChanges <= 6 {
		score += 2.0
	}

	trimmed := strings.TrimSpace(text)
	if strings.HasSuffix(trimmed, ".") || strings.HasSuffix(trimmed, "!") || strings.HasSuffix(trimmed, "?") {
		score += 2.0
	}

	words := len(strings.Fields(text))
	if words >= 20 && words <= 100 {
		score += 2.0
	}
	if words >= 30 && words <= 60 {
		score += 1.0
	}

	return score
}

func suggestTitle(text string) string {
	text = strings.TrimSpace(text)

	// Split into sentences and find the most quotable one
	sentences := splitSentences(text)
	if len(sentences) == 0 {
		if len(text) > 60 {
			if idx := strings.LastIndex(text[:60], " "); idx > 20 {
				return text[:idx] + "..."
			}
			return text[:60] + "..."
		}
		return text
	}

	// Score each sentence for "quotability"
	best := sentences[0]
	bestScore := 0
	for _, s := range sentences {
		s = strings.TrimSpace(s)
		if len(s) < 10 || len(s) > 80 {
			continue
		}
		score := 0
		lower := strings.ToLower(s)
		// Prefer complete sentences
		if strings.HasSuffix(s, ".") || strings.HasSuffix(s, "!") || strings.HasSuffix(s, "?") {
			score += 2
		}
		// Prefer sentences with strong/advice words
		for _, w := range []string{"advice", "key", "secret", "tip", "important", "believe", "love", "strategy"} {
			if strings.Contains(lower, w) {
				score += 3
			}
		}
		// Prefer medium-length sentences (not too short, not too long)
		words := len(strings.Fields(s))
		if words >= 5 && words <= 15 {
			score += 2
		}
		if score > bestScore {
			bestScore = score
			best = s
		}
	}

	best = strings.TrimSpace(best)
	if len(best) > 60 {
		if idx := strings.LastIndex(best[:60], " "); idx > 20 {
			return best[:idx] + "..."
		}
		return best[:60] + "..."
	}
	return best
}

func splitSentences(text string) []string {
	var sentences []string
	var current strings.Builder
	for i, ch := range text {
		current.WriteRune(ch)
		if (ch == '.' || ch == '!' || ch == '?') && i+1 < len(text) && text[i+1] == ' ' {
			s := strings.TrimSpace(current.String())
			if len(s) > 5 {
				sentences = append(sentences, s)
			}
			current.Reset()
		}
	}
	if s := strings.TrimSpace(current.String()); len(s) > 5 {
		sentences = append(sentences, s)
	}
	return sentences
}

func suggestReason(text string, score float64) string {
	lower := strings.ToLower(text)

	if strings.Count(text, "!") >= 2 {
		return "High energy segment with enthusiastic delivery — great for attention-grabbing clips"
	}
	if strings.Count(text, "?") >= 2 {
		return "Contains engaging questions that invite viewer curiosity"
	}

	strongCount := 0
	for _, w := range []string{"absolutely", "definitely", "incredible", "amazing", "important", "critical", "game changer"} {
		if strings.Contains(lower, w) {
			strongCount++
		}
	}
	if strongCount >= 2 {
		return "Strong, confident statements with conviction — works as a standalone soundbite"
	}

	// Check for actionable advice
	for _, w := range []string{"tip", "strategy", "my advice", "what works", "the key is", "here's what", "the secret is"} {
		if strings.Contains(lower, w) {
			return "Contains actionable advice — great for educational social media content"
		}
	}

	words := len(strings.Fields(text))
	if words >= 30 && words <= 60 {
		return "Well-paced segment with good length for a social media clip"
	}

	if score >= 5 {
		return "Engaging content with good energy markers"
	}

	return "Complete thought segment suitable for clipping"
}
