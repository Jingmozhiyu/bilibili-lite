package worker

import (
	"context"
	"sync"
	"time"

	"bilibili-lite/internal/biz"
	"bilibili-lite/internal/conf"

	"github.com/go-kratos/kratos/v3/log"
	"github.com/go-kratos/kratos/v3/transport"
)

var _ transport.Server = (*SearchIndexer)(nil)

// SearchIndexer keeps the optional Meilisearch projection current without blocking app startup.
type SearchIndexer struct {
	videoUsecase  *biz.VideoUsecase
	retryInterval time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewSearchIndexer creates a lifecycle-managed outbox consumer.
func NewSearchIndexer(videoUsecase *biz.VideoUsecase, dataConfig *conf.Data) *SearchIndexer {
	retryInterval := dataConfig.GetSearch().GetRetryInterval().AsDuration()
	if retryInterval <= 0 {
		retryInterval = 5 * time.Second
	}
	return &SearchIndexer{videoUsecase: videoUsecase, retryInterval: retryInterval}
}

// Start waits for Meilisearch to become available, then drains durable updates.
func (w *SearchIndexer) Start(ctx context.Context) error {
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	w.mu.Lock()
	w.cancel = cancel
	w.done = done
	w.mu.Unlock()
	defer close(done)

	ready := false
	for {
		if !ready {
			prepareCtx, prepareCancel := context.WithTimeout(workerCtx, min(10*time.Second, w.retryInterval))
			err := w.videoUsecase.PrepareVideoSearch(prepareCtx)
			prepareCancel()
			if err != nil {
				log.Warn("video search is degraded; PostgreSQL fallback remains active", "error", err)
			} else {
				ready = true
				log.Info("video search index is ready")
			}
		} else {
			processed, err := w.videoUsecase.ProcessNextVideoSearchUpdate(workerCtx)
			if err != nil {
				ready = false
				log.Warn("video search update deferred", "error", err)
			} else if processed {
				continue
			}
		}
		timer := time.NewTimer(w.retryInterval)
		select {
		case <-workerCtx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

// Stop ends retries without waiting for Meilisearch to recover.
func (w *SearchIndexer) Stop(ctx context.Context) error {
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
