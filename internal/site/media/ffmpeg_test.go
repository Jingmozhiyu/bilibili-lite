package media

import (
	"strings"
	"testing"
)

func TestRenditionScaleFilterKeepsWidthEven(t *testing.T) {
	filter := renditionScaleFilter("v1", "vout1", 480)
	if filter != "[v1]scale=-2:480,format=yuv420p,setsar=1[vout1]" {
		t.Fatalf("renditionScaleFilter() = %q", filter)
	}
	if strings.Contains(filter, "force_original_aspect_ratio") {
		t.Fatalf("renditionScaleFilter() may override the even-width -2 expression: %q", filter)
	}
	if !strings.Contains(filter, "format=yuv420p") {
		t.Fatalf("renditionScaleFilter() does not normalize 10-bit sources: %q", filter)
	}
}

func TestMetadataFromProbeUsesRotatedDisplayDimensions(t *testing.T) {
	probe := &probeOutput{Streams: []probeStream{
		{CodecType: "video", Width: 1180, Height: 2556, SideData: []probeSideData{{Rotation: -90}}},
		{CodecType: "audio", SampleRate: "44100", Channels: 2, ChannelLayout: "stereo"},
	}}
	metadata, err := metadataFromProbe(probe)
	if err != nil {
		t.Fatalf("metadataFromProbe() error = %v", err)
	}
	if metadata.Width != 2556 || metadata.Height != 1180 || metadata.Rotation != -90 {
		t.Fatalf("metadataFromProbe() dimensions = %dx%d rotation %d", metadata.Width, metadata.Height, metadata.Rotation)
	}
	renditions := buildRenditions(metadata.Height)
	if got := renditions[len(renditions)-1].Height; got != 1080 {
		t.Fatalf("highest rendition = %d, want 1080", got)
	}
}

func TestFFmpegErrorSummaryKeepsFirstUsefulError(t *testing.T) {
	output := "[scale] Failed to configure output pad\n" + strings.Repeat("encoder statistics\n", 500) + "Conversion failed!"
	summary := ffmpegErrorSummary(output)
	if !strings.Contains(summary, "Failed to configure output pad") {
		t.Fatalf("ffmpegErrorSummary() lost the root error: %q", summary)
	}
	if !strings.Contains(summary, "Conversion failed!") {
		t.Fatalf("ffmpegErrorSummary() lost the final failure: %q", summary)
	}
}

func TestDASHAudioArgsNormalizeSourceAudio(t *testing.T) {
	args := strings.Join(dashAudioArgs(), " ")
	for _, required := range []string{"-map 0:a:0", "-c:a aac", "-b:a 128k", "-ac 2", "-ar 48000"} {
		if !strings.Contains(args, required) {
			t.Errorf("dashAudioArgs() = %q, missing %q", args, required)
		}
	}
}

func TestBuildRenditionsDoesNotUpscale(t *testing.T) {
	tests := []struct {
		name         string
		sourceHeight int32
		wantHeights  []int32
	}{
		{name: "small source", sourceHeight: 240, wantHeights: nil},
		{name: "sub-720p source", sourceHeight: 480, wantHeights: nil},
		{name: "720p source", sourceHeight: 720, wantHeights: []int32{720}},
		{name: "1080p source", sourceHeight: 1080, wantHeights: []int32{720, 1080}},
		{name: "1440p source capped at 1080p", sourceHeight: 1440, wantHeights: []int32{720, 1080}},
		{name: "4k source capped at 1080p", sourceHeight: 2160, wantHeights: []int32{720, 1080}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			renditions := buildRenditions(test.sourceHeight)
			if len(renditions) != len(test.wantHeights) {
				t.Fatalf("buildRenditions(%d) returned %d entries, want %d", test.sourceHeight, len(renditions), len(test.wantHeights))
			}
			for index, rendition := range renditions {
				if rendition.Height != test.wantHeights[index] {
					t.Errorf("rendition %d height = %d, want %d", index, rendition.Height, test.wantHeights[index])
				}
				if rendition.Height > test.sourceHeight {
					t.Errorf("rendition %d upscales %dp source to %dp", index, test.sourceHeight, rendition.Height)
				}
			}
		})
	}
}
