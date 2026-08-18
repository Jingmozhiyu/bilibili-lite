package worker

import (
	"context"
	"sync"
	"time"

	"bilibili-lite/internal/biz"

	"github.com/go-kratos/kratos/v3/log"
	"github.com/go-kratos/kratos/v3/transport"
)

const recommendationRefreshInterval = 30 * time.Second

var _ transport.Server = (*RecommendationRefresher)(nil)

// RecommendationRefresher publishes decayed PostgreSQL engagement scores to Redis.
type RecommendationRefresher struct {
	videoUsecase *biz.VideoUsecase

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewRecommendationRefresher creates the lifecycle-managed leaderboard refresh loop.
func NewRecommendationRefresher(videoUsecase *biz.VideoUsecase) *RecommendationRefresher {
	return &RecommendationRefresher{videoUsecase: videoUsecase}
}

// Start refreshes immediately and then at a fixed interval; Redis outages never stop the app.
func (w *RecommendationRefresher) Start(ctx context.Context) error {
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	w.mu.Lock()
	w.cancel = cancel
	w.done = done
	w.mu.Unlock()
	defer close(done)

	w.refresh(workerCtx)
	ticker := time.NewTicker(recommendationRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-workerCtx.Done():
			return nil
		case <-ticker.C:
			w.refresh(workerCtx)
		}
	}
}

// Stop cancels the refresh loop.
func (w *RecommendationRefresher) Stop(ctx context.Context) error {
	w.mu.Lock()
	cancel := w.cancel
	done := w.done
	w.mu.Unlock()
	if cancel == nil || done == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *RecommendationRefresher) refresh(ctx context.Context) {
	refreshCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := w.videoUsecase.RefreshVideoRanking(refreshCtx); err != nil {
		log.Warn("Redis video ranking is degraded; PostgreSQL fallback remains active", "error", err)
	}
}
