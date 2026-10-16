package data

import (
	"testing"
	"time"
)

func TestVideoHotScoreBalancesFreshnessAndEngagement(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	freshTime := now.Add(-time.Hour)
	oldTime := now.Add(-30 * 24 * time.Hour)
	fresh := videoPO{PublishTime: &freshTime, ViewCount: 100, LikeCount: 10}
	oldEqual := videoPO{PublishTime: &oldTime, ViewCount: 100, LikeCount: 10}
	oldPopular := videoPO{PublishTime: &oldTime, ViewCount: 100_000, LikeCount: 10_000, FavoriteCount: 2_000}

	if videoHotScore(fresh, now) <= videoHotScore(oldEqual, now) {
		t.Fatal("fresh video should outrank an equally engaging old video")
	}
	if videoHotScore(oldPopular, now) <= videoHotScore(fresh, now) {
		t.Fatal("strong engagement should still be able to outrank freshness")
	}
}
