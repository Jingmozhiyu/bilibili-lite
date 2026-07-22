package biz

import "testing"

func TestBVIDRoundTrip(t *testing.T) {
	t.Parallel()

	for _, videoID := range []VideoID{1, 42, VideoID(^uint64(0))} {
		bvid := videoID.BVID()
		parsed, err := ParseBVID(bvid)
		if err != nil {
			t.Fatalf("ParseBVID(%q) returned error: %v", bvid, err)
		}
		if parsed != videoID {
			t.Fatalf("ParseBVID(%q) = %d, want %d", bvid, parsed, videoID)
		}
	}
}

func TestParseBVIDRejectsNonCanonicalValues(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"", "BV", "BV0", "BV01", "bv1", "AV1", "BV-1", "BV1x"} {
		if _, err := ParseBVID(value); err == nil {
			t.Errorf("ParseBVID(%q) unexpectedly succeeded", value)
		}
	}
}

func TestZeroVideoIDHasNoPublicIdentifier(t *testing.T) {
	t.Parallel()

	if bvid := VideoID(0).BVID(); bvid != "" {
		t.Fatalf("VideoID(0).BVID() = %q, want empty string", bvid)
	}
}

func TestNormalizeVideoPageSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   int32
		want    int32
		wantErr bool
	}{
		{name: "default", input: 0, want: defaultVideoPageSize},
		{name: "explicit", input: 12, want: 12},
		{name: "negative", input: -1, wantErr: true},
		{name: "too large", input: maxVideoPageSize + 1, wantErr: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeVideoPageSize(test.input)
			if (err != nil) != test.wantErr {
				t.Fatalf("normalizeVideoPageSize(%d) error = %v, wantErr %v", test.input, err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("normalizeVideoPageSize(%d) = %d, want %d", test.input, got, test.want)
			}
		})
	}
}

func TestValidDanmakuColor(t *testing.T) {
	t.Parallel()

	for _, color := range []string{"#ffffff", "#00A1D6", "#123abc"} {
		if !validDanmakuColor(color) {
			t.Errorf("validDanmakuColor(%q) = false, want true", color)
		}
	}
	for _, color := range []string{"", "ffffff", "#fff", "#12345g", "#1234567"} {
		if validDanmakuColor(color) {
			t.Errorf("validDanmakuColor(%q) = true, want false", color)
		}
	}
}
