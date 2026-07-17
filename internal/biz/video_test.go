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
