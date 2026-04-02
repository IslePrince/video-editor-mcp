package engine

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"video-editor/internal/model"
)

// TranscribeRequest holds everything needed to run a transcription.
type TranscribeRequest struct {
	Job       *model.TranscriptionJob
	InputPath string // path to video/audio file
	OutputDir string // directory to write VTT output
	SaveJob   func(*model.TranscriptionJob) error
}

// DetectGPU checks if CUDA-capable GPU is available for Whisper.
func DetectGPU() bool {
	// Check if nvidia-smi is available and reports a GPU
	out, err := exec.Command("nvidia-smi", "--query-gpu=name", "--format=csv,noheader").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// DetectWhisper checks if whisper CLI is installed.
func DetectWhisper() string {
	// whisper doesn't have --version; check if --help succeeds
	err := exec.Command("whisper", "--help").Run()
	if err != nil {
		return ""
	}
	return "openai-whisper (installed)"
}

// RunTranscription extracts audio from the asset and runs Whisper to produce a VTT file.
func RunTranscription(ctx context.Context, req TranscribeRequest) error {
	job := req.Job

	// Phase 1: Extract audio (0% → 10%)
	job.Status = model.TranscriptionStatusExtracting
	job.Progress = 0.1
	job.UpdatedAt = time.Now().UTC()
	if err := req.SaveJob(job); err != nil {
		return fmt.Errorf("save job: %w", err)
	}

	audioPath := filepath.Join(req.OutputDir, job.ID+".wav")
	if err := extractAudio(ctx, req.InputPath, audioPath); err != nil {
		return fmt.Errorf("extract audio: %w", err)
	}
	defer os.Remove(audioPath) // clean up temp audio

	// Get audio duration for progress calculation
	audioDuration := probeAudioDuration(audioPath)

	// Phase 2: Run Whisper (10% → 95%)
	job.Status = model.TranscriptionStatusTranscribing
	job.Progress = 0.15
	job.UpdatedAt = time.Now().UTC()
	if err := req.SaveJob(job); err != nil {
		return fmt.Errorf("save job: %w", err)
	}

	vttPath, err := runWhisper(ctx, audioPath, req.OutputDir, job, audioDuration)
	if err != nil {
		return fmt.Errorf("whisper: %w", err)
	}

	// Phase 3: Complete
	job.Status = model.TranscriptionStatusComplete
	job.Progress = 1.0
	job.OutputPath = vttPath
	job.UpdatedAt = time.Now().UTC()
	if err := req.SaveJob(job); err != nil {
		return fmt.Errorf("save job: %w", err)
	}

	log.Printf("[transcribe] job %s complete: %s", job.ID, vttPath)
	return nil
}

// probeAudioDuration returns the duration in seconds, or 0 if unknown.
func probeAudioDuration(path string) float64 {
	out, err := exec.Command("ffprobe",
		"-v", "quiet",
		"-show_entries", "format=duration",
		"-of", "csv=p=0",
		path,
	).Output()
	if err != nil {
		return 0
	}
	dur, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0
	}
	return dur
}

// extractAudio uses ffmpeg to extract audio as 16kHz mono WAV (Whisper's preferred format).
func extractAudio(ctx context.Context, inputPath, outputPath string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y",
		"-i", inputPath,
		"-vn",                // no video
		"-acodec", "pcm_s16le", // 16-bit PCM
		"-ar", "16000",       // 16kHz sample rate (Whisper optimal)
		"-ac", "1",           // mono
		outputPath,
	)
	cmd.Stderr = os.Stderr
	log.Printf("[transcribe] extracting audio: %s -> %s", inputPath, outputPath)
	return cmd.Run()
}

// whisperProgressRe matches Whisper's verbose output lines like:
// [00:00.000 --> 00:07.000] Hello world
var whisperProgressRe = regexp.MustCompile(`\[(\d+):(\d+\.\d+)\s*-->`)

// runWhisper invokes the whisper CLI and returns the path to the generated VTT.
func runWhisper(ctx context.Context, audioPath, outputDir string, job *model.TranscriptionJob, audioDuration float64) (string, error) {
	args := []string{
		audioPath,
		"--model", job.Model,
		"--output_format", "vtt",
		"--output_dir", outputDir,
		"--verbose", "True",
	}

	if job.Language != "" {
		args = append(args, "--language", job.Language)
	}

	if !job.UseGPU {
		args = append(args, "--device", "cpu")
	}
	// When UseGPU is true, whisper defaults to CUDA if available

	cmd := exec.CommandContext(ctx, "whisper", args...)
	cmd.Stdout = os.Stdout

	// Capture stderr to parse progress from Whisper's verbose output
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("stderr pipe: %w", err)
	}

	log.Printf("[transcribe] running whisper: whisper %s", strings.Join(args, " "))
	if err := cmd.Start(); err != nil {
		return "", err
	}

	// Parse Whisper output for progress updates in background
	go trackWhisperProgress(stderrPipe, job, audioDuration)

	if err := cmd.Wait(); err != nil {
		return "", err
	}

	// Whisper outputs: <basename>.vtt in the output dir
	baseName := strings.TrimSuffix(filepath.Base(audioPath), filepath.Ext(audioPath))
	vttPath := filepath.Join(outputDir, baseName+".vtt")

	if _, err := os.Stat(vttPath); err != nil {
		return "", fmt.Errorf("expected VTT output not found at %s", vttPath)
	}

	// Rename to a cleaner name based on job ID
	finalPath := filepath.Join(outputDir, job.ID+".vtt")
	if vttPath != finalPath {
		if err := os.Rename(vttPath, finalPath); err != nil {
			return vttPath, nil // use original if rename fails
		}
		return finalPath, nil
	}
	return vttPath, nil
}

// trackWhisperProgress reads Whisper's stderr and updates job progress.
func trackWhisperProgress(r io.Reader, job *model.TranscriptionJob, audioDuration float64) {
	scanner := bufio.NewScanner(r)
	lastUpdate := time.Now()
	for scanner.Scan() {
		line := scanner.Text()
		// Also write to stderr so logs are preserved
		fmt.Fprintln(os.Stderr, line)

		if audioDuration <= 0 {
			continue
		}

		matches := whisperProgressRe.FindStringSubmatch(line)
		if len(matches) < 3 {
			continue
		}

		// Only update every 3 seconds to avoid excessive saves
		if time.Since(lastUpdate) < 3*time.Second {
			continue
		}

		minutes, _ := strconv.ParseFloat(matches[1], 64)
		seconds, _ := strconv.ParseFloat(matches[2], 64)
		currentTime := minutes*60 + seconds

		// Map progress to 0.15 → 0.95 range (extraction is 0→0.15, finalization is 0.95→1.0)
		rawProgress := currentTime / audioDuration
		if rawProgress > 1 {
			rawProgress = 1
		}
		job.Progress = 0.15 + rawProgress*0.80
		job.UpdatedAt = time.Now().UTC()
		lastUpdate = time.Now()
	}
}
