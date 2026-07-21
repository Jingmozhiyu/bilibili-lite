package data

import (
	"database/sql"
	"testing"
	"time"
)

func TestVideoPageTokenRoundTrip(t *testing.T) {
	for _, videoID := range []uint64{1, 42, ^uint64(0)} {
		token := encodeVideoPageToken(videoID)
		got, err := decodeVideoPageToken(token)
		if err != nil {
			t.Fatalf("decodeVideoPageToken(%q) error = %v", token, err)
		}
		if got != videoID {
			t.Fatalf("decodeVideoPageToken(%q) = %d, want %d", token, got, videoID)
		}
	}
	if _, err := decodeVideoPageToken("not-a-token"); err == nil {
		t.Fatal("invalid page token unexpectedly succeeded")
	}
}

func TestBuildVideoViewResultUsesLatestLimit(t *testing.T) {
	now := time.Date(2026, 7, 19, 10, 0, 0, 0, shanghaiTime)
	result := buildVideoViewResult(false, 12, maxDailyVideoViews, sql.NullTime{
		Time: now.Add(-30 * time.Minute), Valid: true,
	}, now)
	if result.RemainingToday != 0 {
		t.Fatalf("RemainingToday = %d, want 0", result.RemainingToday)
	}
	want := time.Date(2026, 7, 20, 0, 0, 0, 0, shanghaiTime)
	if !result.NextEligibleAt.Equal(want) {
		t.Fatalf("NextEligibleAt = %v, want %v", result.NextEligibleAt, want)
	}
}

func TestMigrationVersion(t *testing.T) {
	version, err := migrationVersion("000002_video_lifecycle.sql")
	if err != nil || version != 2 {
		t.Fatalf("migrationVersion() = %d, %v, want 2", version, err)
	}
	if _, err := migrationVersion("video.sql"); err == nil {
		t.Fatal("migration without numeric prefix unexpectedly succeeded")
	}
}
