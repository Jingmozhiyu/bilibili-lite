package data

import (
	"testing"
	"time"

	"bilibili-lite/internal/biz"
)

func TestProcessingSubmissionEntersReviewAfterTranscode(t *testing.T) {
	now := time.Now()
	if status := videoStatusAfterTranscode(&now); status != biz.VideoStatusPendingReview {
		t.Fatalf("videoStatusAfterTranscode(submitted) = %q", status)
	}
	if status := videoStatusAfterTranscode(nil); status != biz.VideoStatusReady {
		t.Fatalf("videoStatusAfterTranscode(draft) = %q", status)
	}
}

func TestCanSubmitVideoStatus(t *testing.T) {
	for _, status := range []biz.VideoStatus{biz.VideoStatusProcessing, biz.VideoStatusReady, biz.VideoStatusRejected} {
		if !canSubmitVideoStatus(status) {
			t.Errorf("canSubmitVideoStatus(%q) = false", status)
		}
	}
	for _, status := range []biz.VideoStatus{biz.VideoStatusPendingReview, biz.VideoStatusPublished, biz.VideoStatusFailed, biz.VideoStatusDeleted} {
		if canSubmitVideoStatus(status) {
			t.Errorf("canSubmitVideoStatus(%q) = true", status)
		}
	}
}
