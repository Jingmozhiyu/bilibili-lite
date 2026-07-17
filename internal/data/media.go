package data

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bilibili-lite/internal/biz"

	"github.com/go-kratos/kratos/v3/log"
	"gorm.io/gorm"
)

var errUploadTooLarge = errors.New("upload too large")

// probeOutput is the subset of ffprobe JSON used to persist playback metadata.
type probeOutput struct {
	Format struct {
		Duration string `json:"duration"`
		BitRate  string `json:"bit_rate"`
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		Width     int32  `json:"width"`
		Height    int32  `json:"height"`
		BitRate   string `json:"bit_rate"`
	} `json:"streams"`
}

// PublishVideoFromMP4 receives an upload, transcodes it to DASH, publishes its files, and persists metadata.
func (r *videoRepo) PublishVideoFromMP4(ctx context.Context, input *biz.VideoUploadInput) (*biz.VideoUploadResult, error) {
	jobID, err := newJobID()
	if err != nil {
		return nil, biz.ErrVideoStorage
	}
	jobDir := filepath.Join(r.data.mediaRoot, ".uploads", jobID)
	outputDir := filepath.Join(jobDir, "output")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, biz.ErrVideoStorage
	}
	if err := touchHeartbeat(jobDir); err != nil {
		return nil, biz.ErrVideoStorage
	}

	sourcePath := filepath.Join(jobDir, "source.mp4.part")
	if err := copyUpload(ctx, sourcePath, input.Content, r.data.maxUploadBytes, jobDir); err != nil {
		if errors.Is(err, errUploadTooLarge) {
			return nil, biz.ErrVideoUploadTooLarge
		}
		return nil, biz.ErrVideoUploadInterrupted
	}

	probe, err := inspectMP4(ctx, sourcePath)
	if err != nil {
		return nil, biz.ErrVideoProcessing
	}
	if err := transcodeDASH(ctx, sourcePath, outputDir, jobDir, r.data.transcodeTimeout); err != nil {
		return nil, biz.ErrVideoProcessing
	}

	result, err := r.persistAndPublishUploadedVideo(ctx, input, probe, outputDir)
	if err != nil {
		log.Error("publish uploaded video", "error", err)
		return nil, err
	}
	_ = os.RemoveAll(jobDir)
	return result, nil
}

// copyUpload drains the HTTP-backed reader into a temporary file while enforcing size and idle cleanup state.
func copyUpload(ctx context.Context, path string, uploadStream io.Reader, limit int64, jobDir string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	buffer := make([]byte, 1024*1024)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := uploadStream.Read(buffer)
		if n > 0 {
			written += int64(n)
			if written > limit {
				return errUploadTooLarge
			}
			if _, err := file.Write(buffer[:n]); err != nil {
				return err
			}
			if err := touchHeartbeat(jobDir); err != nil {
				return err
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return file.Sync()
			}
			return readErr
		}
	}
}

// inspectMP4 uses ffprobe to collect media metadata and require both video and audio tracks.
func inspectMP4(ctx context.Context, path string) (*probeOutput, error) {
	probePath, err := exec.LookPath("ffprobe")
	if err != nil {
		return nil, fmt.Errorf("ffprobe is not installed: %w", err)
	}
	commandCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, probePath,
		"-v", "error", "-print_format", "json", "-show_format", "-show_streams", path,
	).Output()
	if err != nil {
		return nil, err
	}
	var probe probeOutput
	if err := json.Unmarshal(output, &probe); err != nil {
		return nil, err
	}
	var hasVideo, hasAudio bool
	for _, stream := range probe.Streams {
		hasVideo = hasVideo || stream.CodecType == "video"
		hasAudio = hasAudio || stream.CodecType == "audio"
	}
	if !hasVideo || !hasAudio {
		return nil, fmt.Errorf("MP4 must contain one video and one audio stream")
	}
	return &probe, nil
}

// transcodeDASH runs FFmpeg to produce four-second video and audio segments plus an MPD manifest.
func transcodeDASH(ctx context.Context, sourcePath, outputDir, jobDir string, timeout time.Duration) error {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("ffmpeg is not installed: %w", err)
	}
	transcodeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	manifestPath := filepath.Join(outputDir, "manifest.mpd")
	cmd := exec.CommandContext(transcodeCtx, ffmpegPath,
		"-hide_banner", "-y", "-i", sourcePath,
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
		_ = touchHeartbeat(jobDir)
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

// persistAndPublishUploadedVideo obtains the auto-increment ID, publishes DASH files, and inserts the stream atomically.
func (r *videoRepo) persistAndPublishUploadedVideo(ctx context.Context, input *biz.VideoUploadInput, probe *probeOutput, outputDir string) (*biz.VideoUploadResult, error) {
	duration, _ := strconv.ParseFloat(probe.Format.Duration, 64)
	bandwidth, _ := strconv.ParseInt(probe.Format.BitRate, 10, 32)
	var width, height int32
	for _, stream := range probe.Streams {
		if stream.CodecType == "video" {
			width, height = stream.Width, stream.Height
			if streamBandwidth, err := strconv.ParseInt(stream.BitRate, 10, 32); err == nil && streamBandwidth > bandwidth {
				bandwidth = streamBandwidth
			}
			break
		}
	}
	if bandwidth > math.MaxInt32 {
		bandwidth = math.MaxInt32
	}
	record := videoPO{
		OwnerID: input.OwnerID, Title: input.Title,
		Description: input.Description, DurationSeconds: int64(math.Ceil(duration)),
		PublishTime: time.Now(), Tags: append([]string(nil), input.Tags...),
	}
	var result *biz.VideoUploadResult
	var publishedDir string
	if err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		videoID := biz.VideoID(record.ID)
		bvid := videoID.BVID()
		finalDir := filepath.Join(r.data.mediaRoot, "dash", bvid)
		if err := os.Rename(outputDir, finalDir); err != nil {
			return fmt.Errorf("publish DASH directory for %s: %w", bvid, err)
		}
		publishedDir = finalDir
		manifestURL := "/media/dash/" + bvid + "/manifest.mpd"
		stream := videoStreamPO{
			VideoID: record.ID, StreamKey: "dash-main", Label: "DASH",
			Codec: "avc1,mp4a", MimeType: "application/dash+xml", URL: manifestURL,
			Width: width, Height: height, Bandwidth: int32(bandwidth), DefaultStream: true,
		}
		result = &biz.VideoUploadResult{VideoID: videoID, ManifestURL: manifestURL}
		return tx.Create(&stream).Error
	}); err != nil {
		if publishedDir != "" {
			if cleanupErr := os.RemoveAll(publishedDir); cleanupErr != nil {
				log.Error("remove rolled-back DASH directory", "path", publishedDir, "error", cleanupErr)
			}
		}
		return nil, biz.ErrVideoStorage
	}
	return result, nil
}

// newJobID generates an unpredictable directory name for an in-progress upload.
func newJobID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// tail bounds FFmpeg diagnostics while preserving the most recent output.
func tail(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[len(value)-max:]
}
