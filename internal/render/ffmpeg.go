package render

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"video-editor/internal/engine"
	"video-editor/internal/model"
	"video-editor/internal/storage"
)

type RenderExecutor struct {
	store storage.Storage
}

func NewRenderExecutor(store storage.Storage) *RenderExecutor {
	return &RenderExecutor{store: store}
}

func (e *RenderExecutor) Execute(ctx context.Context, job *model.RenderJob, fg *engine.FilterGraph) error {
	outputPath := e.store.RenderOutputPath(job.ProjectID, job.ID)
	job.OutputPath = outputPath

	args := buildFFmpegArgs(fg, job.Profile, outputPath)
	job.Command = "ffmpeg " + strings.Join(args, " ")
	job.FilterGraph = fg.FilterChain

	// Update status to rendering
	job.Status = model.RenderStatusRendering
	job.UpdatedAt = time.Now().UTC()
	e.store.SaveRenderJob(job)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	log.Printf("[render:%s] starting ffmpeg", job.ID)
	log.Printf("[render:%s] command: ffmpeg %s", job.ID, strings.Join(args, " "))
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}

	// Parse progress from stderr and capture errors
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 64*1024), 256*1024)
	scanner.Split(scanFFmpegLines)
	progressRe := regexp.MustCompile(`time=(\d+:\d+:\d+\.\d+)`)
	var lastLines []string

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 0 {
			// Keep last 20 lines for error reporting
			lastLines = append(lastLines, line)
			if len(lastLines) > 20 {
				lastLines = lastLines[1:]
			}
		}
		if matches := progressRe.FindStringSubmatch(line); len(matches) > 1 {
			log.Printf("[render:%s] progress time=%s", job.ID, matches[1])
		}
	}

	if err := cmd.Wait(); err != nil {
		errOutput := strings.Join(lastLines, "\n")
		log.Printf("[render:%s] ffmpeg stderr:\n%s", job.ID, errOutput)
		return fmt.Errorf("ffmpeg failed: %w\nOutput:\n%s", err, errOutput)
	}

	// Finalize
	job.Status = model.RenderStatusComplete
	job.Progress = 1.0
	job.UpdatedAt = time.Now().UTC()
	e.store.SaveRenderJob(job)

	log.Printf("[render:%s] complete: %s", job.ID, outputPath)
	return nil
}

func buildFFmpegArgs(fg *engine.FilterGraph, profile model.RenderProfile, outputPath string) []string {
	args := []string{"-y"}

	// Input files
	for _, input := range fg.Inputs {
		args = append(args, "-i", input.Path)
	}

	// Filter complex
	args = append(args, "-filter_complex", fg.FilterChain)

	// Map outputs
	args = append(args, "-map", fg.VideoOut, "-map", fg.AudioOut)

	// Video codec
	args = append(args, "-c:v", profile.VideoCodec)
	if profile.VideoCodec == "libx264" {
		args = append(args, "-profile:v", "high", "-pix_fmt", "yuv420p", "-preset", "fast")
	}
	if profile.Quality > 0 {
		args = append(args, "-crf", strconv.Itoa(profile.Quality))
	}
	if profile.Width > 0 && profile.Height > 0 {
		args = append(args, "-s", fmt.Sprintf("%dx%d", profile.Width, profile.Height))
	}

	// Audio codec
	args = append(args, "-c:a", profile.AudioCodec, "-b:a", "128k")

	// faststart for web
	args = append(args, "-movflags", "+faststart")

	args = append(args, outputPath)
	return args
}

// scanFFmpegLines splits on \r or \n for ffmpeg progress output
func scanFFmpegLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' || data[i] == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}
