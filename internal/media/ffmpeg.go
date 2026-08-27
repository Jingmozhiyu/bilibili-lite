package media

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ErrVideoResolutionTooLow reports that the displayed source height cannot
// satisfy the minimum 720p output policy without upscaling.
var ErrVideoResolutionTooLow = errors.New("video display height must be at least 720p")

// ErrTranscodeTimeout reports that FFmpeg exceeded the configured wall-clock budget.
var ErrTranscodeTimeout = errors.New("video transcode timed out")

// Metadata contains the ffprobe values persisted for one DASH stream.
type Metadata struct {
	DurationSeconds int64
	Width           int32
	Height          int32
	Bandwidth       int32
	VideoCodec      string
	PixelFormat     string
	FrameRate       string
	AudioCodec      string
	AudioSampleRate string
	AudioChannels   int32
	AudioLayout     string
	Rotation        int32
}

// Rendition describes one encoded DASH video representation.
type Rendition struct {
	Height    int32
	Bandwidth int32
}

type probeSideData struct {
	Rotation int32 `json:"rotation"`
}

type probeStream struct {
	CodecType     string          `json:"codec_type"`
	CodecName     string          `json:"codec_name"`
	Width         int32           `json:"width"`
	Height        int32           `json:"height"`
	BitRate       string          `json:"bit_rate"`
	PixelFormat   string          `json:"pix_fmt"`
	FrameRate     string          `json:"avg_frame_rate"`
	SampleRate    string          `json:"sample_rate"`
	Channels      int32           `json:"channels"`
	ChannelLayout string          `json:"channel_layout"`
	SideData      []probeSideData `json:"side_data_list"`
}

type probeOutput struct {
	Format struct {
		Duration string `json:"duration"`
		BitRate  string `json:"bit_rate"`
	} `json:"format"`
	Streams []probeStream `json:"streams"`
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

// TranscodeDASH creates a no-upscale bitrate ladder and one adaptive DASH manifest.
func (m *Manager) TranscodeDASH(ctx context.Context, job *UploadJob, metadata *Metadata) ([]Rendition, error) {
	if job == nil || metadata == nil {
		return nil, fmt.Errorf("upload job and metadata are required")
	}
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("ffmpeg is not installed: %w", err)
	}
	renditions := buildRenditions(metadata.Height)
	if len(renditions) == 0 {
		return nil, ErrVideoResolutionTooLow
	}
	transcodeCtx, cancel := context.WithTimeout(ctx, m.transcodeTimeout)
	defer cancel()
	manifestPath := filepath.Join(job.outputDir, "manifest.mpd")
	args := []string{"-hide_banner", "-y", "-i", job.sourcePath}
	filterOutputs := make([]string, len(renditions))
	if len(renditions) == 1 {
		filterOutputs[0] = renditionScaleFilter("0:v", "vout0", renditions[0].Height)
	} else {
		filterInputs := make([]string, len(renditions))
		for index, rendition := range renditions {
			filterInputs[index] = fmt.Sprintf("[v%d]", index)
			filterOutputs[index] = renditionScaleFilter(fmt.Sprintf("v%d", index), fmt.Sprintf("vout%d", index), rendition.Height)
		}
		filterOutputs[0] = fmt.Sprintf("[0:v]split=%d%s;%s", len(renditions), strings.Join(filterInputs, ""), filterOutputs[0])
	}
	filter := strings.Join(filterOutputs, ";")
	args = append(args, "-filter_complex", filter)
	for index, rendition := range renditions {
		args = append(args,
			"-map", fmt.Sprintf("[vout%d]", index),
			fmt.Sprintf("-b:v:%d", index), strconv.FormatInt(int64(rendition.Bandwidth), 10),
			fmt.Sprintf("-maxrate:v:%d", index), strconv.FormatInt(int64(rendition.Bandwidth*12/10), 10),
			fmt.Sprintf("-bufsize:v:%d", index), strconv.FormatInt(int64(rendition.Bandwidth*2), 10),
		)
	}
	args = append(args,
		"-c:v", "libx264", "-preset", "veryfast",
		"-g", "48", "-keyint_min", "48", "-sc_threshold", "0",
	)
	args = append(args, dashAudioArgs()...)
	args = append(args,
		"-use_template", "1", "-use_timeline", "1", "-seg_duration", "4",
		"-adaptation_sets", "id=0,streams=v id=1,streams=a",
		"-init_seg_name", "init-$RepresentationID$.m4s",
		"-media_seg_name", "chunk-$RepresentationID$-$Number%05d$.m4s",
		"-progress", "pipe:1", "-nostats", "-f", "dash", manifestPath,
	)
	cmd := exec.CommandContext(transcodeCtx, ffmpegPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		_ = job.touchHeartbeat()
	}
	if err := scanner.Err(); err != nil {
		cancel()
		_ = cmd.Wait()
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		if errors.Is(transcodeCtx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w after %s", ErrTranscodeTimeout, m.transcodeTimeout)
		}
		return nil, fmt.Errorf("ffmpeg: %w: %s", err, ffmpegErrorSummary(stderr.String()))
	}
	return renditions, nil
}

func ffmpegErrorSummary(output string) string {
	// With -nostats, stderr contains command setup and the final encoder report,
	// not per-frame progress. Keep enough of the tail to retain muxer context
	// that often appears several lines before the generic Conversion failed.
	return tail(output, 64*1024)
}

func dashAudioArgs() []string {
	// Normalize arbitrary upload audio before DASH packaging. In particular,
	// native AAC cannot initialize every source sample-rate/channel-layout
	// combination accepted by MP4, which otherwise fails the final mapped
	// output stream while all video renditions are valid.
	return []string{"-map", "0:a:0", "-c:a", "aac", "-b:a", "128k", "-ac", "2", "-ar", "48000"}
}

func renditionScaleFilter(input, output string, height int32) string {
	// The ladder only contains heights at or below the source height, so a
	// fixed target height cannot upscale. Let scale calculate an even width;
	// force_original_aspect_ratio=decrease can override -2 and produce odd
	// widths such as 853x480, which libx264 rejects for yuv420p output.
	// DASH requires every representation in one adaptation set to report the
	// same sample aspect ratio. Rounding -2 to an even width can otherwise
	// leave different SAR values for non-standard source ratios.
	return fmt.Sprintf("[%s]scale=-2:%d,format=yuv420p,setsar=1[%s]", input, height, output)
}

func buildRenditions(sourceHeight int32) []Rendition {
	profiles := []Rendition{
		{Height: 720, Bandwidth: 2_800_000},
		{Height: 1080, Bandwidth: 5_000_000},
	}
	result := make([]Rendition, 0, len(profiles))
	for _, profile := range profiles {
		if profile.Height <= sourceHeight {
			result = append(result, profile)
		}
	}
	return result
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
			metadata.Width, metadata.Height, metadata.Rotation = displayDimensions(stream)
			metadata.VideoCodec = stream.CodecName
			metadata.PixelFormat = stream.PixelFormat
			metadata.FrameRate = stream.FrameRate
			if streamBandwidth, err := strconv.ParseInt(stream.BitRate, 10, 32); err == nil && streamBandwidth > bandwidth {
				bandwidth = streamBandwidth
			}
		case "audio":
			hasAudio = true
			metadata.AudioCodec = stream.CodecName
			metadata.AudioSampleRate = stream.SampleRate
			metadata.AudioChannels = stream.Channels
			metadata.AudioLayout = stream.ChannelLayout
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

func displayDimensions(stream probeStream) (int32, int32, int32) {
	rotation := int32(0)
	for _, sideData := range stream.SideData {
		if sideData.Rotation != 0 {
			rotation = sideData.Rotation
			break
		}
	}
	normalized := ((rotation % 360) + 360) % 360
	if normalized == 90 || normalized == 270 {
		return stream.Height, stream.Width, rotation
	}
	return stream.Width, stream.Height, rotation
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
