package worker

import (
	"context"
	"sync"
	"time"

	"bilibili-lite/internal/biz"
	"bilibili-lite/internal/media"

	"github.com/go-kratos/kratos/v3/log"
	"github.com/go-kratos/kratos/v3/transport"
)

const uploadCleanupInterval = 5 * time.Second

var _ transport.Server = (*UploadJanitor)(nil)

// UploadJanitor runs stale-upload cleanup as a Kratos-managed background server.
type UploadJanitor struct {
	mediaManager *media.Manager
	videoUsecase *biz.VideoUsecase
	interval     time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewUploadJanitor creates the lifecycle-managed stale-upload worker.
func NewUploadJanitor(mediaManager *media.Manager, videoUsecase *biz.VideoUsecase) *UploadJanitor {
	worker := newUploadJanitor(mediaManager, uploadCleanupInterval)
	worker.videoUsecase = videoUsecase
	return worker
}

// Start blocks while periodically removing upload jobs with expired heartbeats.
func (w *UploadJanitor) Start(ctx context.Context) error {
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	w.mu.Lock()
	w.cancel = cancel
	w.done = done
	w.mu.Unlock()
	defer close(done)

	w.cleanup()
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-workerCtx.Done():
			return nil
		case <-ticker.C:
			w.cleanup()
		}
	}
}

// Stop cancels the cleanup loop and waits for its Start method to return.
func (w *UploadJanitor) Stop(ctx context.Context) error {
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

func newUploadJanitor(mediaManager *media.Manager, interval time.Duration) *UploadJanitor {
	return &UploadJanitor{mediaManager: mediaManager, interval: interval}
}

func (w *UploadJanitor) cleanup() {
	active := make(map[string]struct{})
	if w.videoUsecase != nil {
		jobIDs, err := w.videoUsecase.ActiveUploadJobIDs(context.Background())
		if err != nil {
			log.Error("list active upload jobs", "error", err)
			return
		}
		for _, jobID := range jobIDs {
			active[jobID] = struct{}{}
		}
	}
	removed, err := w.mediaManager.CleanupStaleUploads(active)
	if err != nil {
		log.Error("clean stale uploads", "error", err)
	}
	if removed > 0 {
		log.Info("removed stale uploads", "count", removed)
	}
}
