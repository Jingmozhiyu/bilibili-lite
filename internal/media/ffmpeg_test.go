package media

import "testing"

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
