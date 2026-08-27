package media

import (
	"strings"
	"testing"
)

func TestRenditionScaleFilterKeepsWidthEven(t *testing.T) {
	filter := renditionScaleFilter("v1", "vout1", 480)
	if filter != "[v1]scale=-2:480[vout1]" {
		t.Fatalf("renditionScaleFilter() = %q", filter)
	}
	if strings.Contains(filter, "force_original_aspect_ratio") {
		t.Fatalf("renditionScaleFilter() may override the even-width -2 expression: %q", filter)
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

func TestBuildRenditionsDoesNotUpscale(t *testing.T) {
	tests := []struct {
		name         string
		sourceHeight int32
		wantHeights  []int32
	}{
		{name: "small source", sourceHeight: 240, wantHeights: []int32{240}},
		{name: "720p source", sourceHeight: 720, wantHeights: []int32{360, 480, 720}},
		{name: "1080p source", sourceHeight: 1080, wantHeights: []int32{360, 480, 720, 1080}},
		{name: "4k source", sourceHeight: 2160, wantHeights: []int32{360, 480, 720, 1080, 1440, 2160}},
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
