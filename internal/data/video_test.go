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

func TestVideoHistoryTokenRoundTrip(t *testing.T) {
	t.Parallel()

	want := videoHistoryCursor{
		UpdatedAt: time.Date(2026, 7, 22, 12, 34, 56, 789, time.UTC),
		ID:        42,
	}
	token := encodeVideoHistoryToken(want)
	got, err := decodeVideoHistoryToken(token)
	if err != nil {
		t.Fatalf("decodeVideoHistoryToken(%q) error = %v", token, err)
	}
	if !got.UpdatedAt.Equal(want.UpdatedAt) || got.ID != want.ID {
		t.Fatalf("decodeVideoHistoryToken(%q) = %+v, want %+v", token, got, want)
	}
	for _, token := range []string{"invalid", "MTIz", "MTIzOjA"} {
		if _, err := decodeVideoHistoryToken(token); err == nil {
			t.Errorf("decodeVideoHistoryToken(%q) unexpectedly succeeded", token)
		}
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

func TestRowToBizVideoCommentPreservesReplyTarget(t *testing.T) {
	t.Parallel()
	rootID := uint64(4)
	parentID := uint64(5)
	replyUserID := uint64(2)
	comment := rowToBizVideoComment(videoCommentRow{
		ID: 6, VideoID: 4, UserID: 1, UserName: "作者",
		RootID: &rootID, ParentID: &parentID, ReplyToUserID: &replyUserID,
		ReplyToUserName: "楼中楼成员", Content: "回复内容", LikeCount: 3, Liked: true,
	})
	if comment.RootID != rootID || comment.ParentID != parentID || comment.ReplyToUserID != replyUserID {
		t.Fatalf("rowToBizVideoComment() relationships = %+v", comment)
	}
	if comment.ReplyToUserName != "楼中楼成员" || !comment.Liked || comment.LikeCount != 3 {
		t.Fatalf("rowToBizVideoComment() interaction fields = %+v", comment)
	}
}

func TestRowToBizVideoCommentHidesDeletedContent(t *testing.T) {
	t.Parallel()
	now := time.Now()
	comment := rowToBizVideoComment(videoCommentRow{ID: 1, Content: "不应返回", DeletedAt: &now})
	if !comment.Deleted || comment.Content != "" {
		t.Fatalf("rowToBizVideoComment() = %+v, want deleted comment without content", comment)
	}
}
