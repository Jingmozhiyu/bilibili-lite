package data

import (
	"testing"
	"time"
)

func TestAvailableExperienceAward(t *testing.T) {
	tests := []struct {
		name                      string
		current, requested, limit int32
		want                      int32
	}{
		{name: "first daily award", requested: 5, limit: 5, want: 5},
		{name: "daily source already awarded", current: 5, requested: 5, limit: 5, want: 0},
		{name: "coin award reaches cap", current: 40, requested: 20, limit: 50, want: 10},
		{name: "coin award below cap", current: 20, requested: 10, limit: 50, want: 10},
		{name: "nonpositive request", requested: 0, limit: 50, want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := availableExperienceAward(test.current, test.requested, test.limit); got != test.want {
				t.Fatalf("availableExperienceAward(%d, %d, %d) = %d, want %d", test.current, test.requested, test.limit, got, test.want)
			}
		})
	}
}

func TestShanghaiExperienceDate(t *testing.T) {
	beforeMidnight := time.Date(2026, time.August, 3, 15, 59, 59, 0, time.UTC)
	afterMidnight := beforeMidnight.Add(time.Second)
	if got := shanghaiExperienceDate(beforeMidnight); got != "2026-08-03" {
		t.Fatalf("date before Shanghai midnight = %q, want 2026-08-03", got)
	}
	if got := shanghaiExperienceDate(afterMidnight); got != "2026-08-04" {
		t.Fatalf("date after Shanghai midnight = %q, want 2026-08-04", got)
	}
}
