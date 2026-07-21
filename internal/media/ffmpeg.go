package media

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Metadata contains the ffprobe values persisted for one DASH stream.
type Metadata struct {
	DurationSeconds int64
	Width           int32
	Height          int32
	Bandwidth       int32
}

type probeOutput struct {
	Format struct {
		Duration string `json:"duration"`
		BitRate  string `json:"bit_rate"`
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		Width     int32  `json:"width"`
		Height    int32  `json:"height"`
		BitRate   string `json:"bit_rate"`
	} `json:"streams"`
}

// InspectMP4 runs ffprobe and requires one video track and one audio track.
func (m *Manager) InspectMP4(ctx context.Context, job *UploadJob) (*Metadata, error) {
	if job == nil {
		return nil, fmt.Errorf("upload job is required")
	}
	probePath, err := exec.LookPath("ffprobe")
	if err != nil {
		return nil, fmt.Errorf("ffprobe is not installed: %w", err)
	}
	commandCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, probePath,
		"-v", "error", "-print_format", "json", "-show_format", "-show_streams", job.sourcePath,
	).Output()
	if err != nil {
		return nil, err
	}
	var probe probeOutput
	if err := json.Unmarshal(output, &probe); err != nil {
		return nil, err
	}
	return metadataFromProbe(&probe)
}

// GenerateCover normalizes a custom image or captures a random video frame as cover.jpg.
func (m *Manager) GenerateCover(ctx context.Context, job *UploadJob, metadata *Metadata, custom bool) error {
	if job == nil || metadata == nil {
		return fmt.Errorf("upload job and metadata are required")
	}
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("ffmpeg is not installed: %w", err)
	}
	commandCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	args := []string{"-hide_banner", "-y"}
	if custom {
		args = append(args, "-i", job.coverPath)
	} else {
		args = append(args, "-ss", randomCoverTimestamp(metadata.DurationSeconds), "-i", job.sourcePath)
	}
	args = append(args,
		"-frames:v", "1",
		"-vf", "scale=1280:-2:force_original_aspect_ratio=decrease",
		"-q:v", "2", filepath.Join(job.outputDir, "cover.jpg"),
	)
	output, err := exec.CommandContext(commandCtx, ffmpegPath, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("generate cover: %w: %s", err, tail(string(output), 1200))
	}
	return job.touchHeartbeat()
}

// TranscodeDASH runs FFmpeg to produce four-second video/audio segments and an MPD manifest.
func (m *Manager) TranscodeDASH(ctx context.Context, job *UploadJob) error {
	if job == nil {
		return fmt.Errorf("upload job is required")
	}
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("ffmpeg is not installed: %w", err)
	}
	transcodeCtx, cancel := context.WithTimeout(ctx, m.transcodeTimeout)
	defer cancel()
	manifestPath := filepath.Join(job.outputDir, "manifest.mpd")
	cmd := exec.CommandContext(transcodeCtx, ffmpegPath,
		"-hide_banner", "-y", "-i", job.sourcePath,
		"-map", "0:v:0", "-map", "0:a:0",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "23",
		"-g", "48", "-keyint_min", "48", "-sc_threshold", "0",
		"-c:a", "aac", "-b:a", "128k",
		"-use_template", "1", "-use_timeline", "1", "-seg_duration", "4",
		"-adaptation_sets", "id=0,streams=v id=1,streams=a",
		"-progress", "pipe:1", "-nostats", "-f", "dash", manifestPath,
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		_ = job.touchHeartbeat()
	}
	if err := scanner.Err(); err != nil {
		cancel()
		_ = cmd.Wait()
		return err
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg: %w: %s", err, tail(stderr.String(), 1200))
	}
	return nil
}

func metadataFromProbe(probe *probeOutput) (*Metadata, error) {
	duration, _ := strconv.ParseFloat(probe.Format.Duration, 64)
	bandwidth, _ := strconv.ParseInt(probe.Format.BitRate, 10, 32)
	metadata := &Metadata{DurationSeconds: int64(math.Ceil(duration))}
	var hasVideo, hasAudio bool
	for _, stream := range probe.Streams {
		switch stream.CodecType {
		case "video":
			hasVideo = true
			metadata.Width = stream.Width
			metadata.Height = stream.Height
			if streamBandwidth, err := strconv.ParseInt(stream.BitRate, 10, 32); err == nil && streamBandwidth > bandwidth {
				bandwidth = streamBandwidth
			}
		case "audio":
			hasAudio = true
		}
	}
	if !hasVideo || !hasAudio {
		return nil, fmt.Errorf("MP4 must contain one video and one audio stream")
	}
	if bandwidth > math.MaxInt32 {
		bandwidth = math.MaxInt32
	}
	metadata.Bandwidth = int32(bandwidth)
	return metadata, nil
}

func tail(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[len(value)-max:]
}

func randomCoverTimestamp(durationSeconds int64) string {
	if durationSeconds <= 1 {
		return "0"
	}
	value, err := rand.Int(rand.Reader, big.NewInt(durationSeconds))
	if err != nil {
		return strconv.FormatInt(durationSeconds/2, 10)
	}
	return value.String()
}
