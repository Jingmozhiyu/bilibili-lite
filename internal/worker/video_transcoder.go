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

var _ transport.Server = (*VideoTranscoder)(nil)

// VideoTranscoder bounds concurrent FFmpeg work and retries abandoned claims after restart.
type VideoTranscoder struct {
	videoUsecase *biz.VideoUsecase
	workers      int
	pollInterval time.Duration
	staleAfter   time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewVideoTranscoder creates the lifecycle-managed media worker pool.
func NewVideoTranscoder(videoUsecase *biz.VideoUsecase, dataConfig *conf.Data) *VideoTranscoder {
	mediaConfig := dataConfig.GetMedia()
	return &VideoTranscoder{
		videoUsecase: videoUsecase,
		workers:      int(mediaConfig.GetTranscodeWorkers()),
		pollInterval: mediaConfig.GetTranscodePollInterval().AsDuration(),
		staleAfter:   mediaConfig.GetTranscodeTimeout().AsDuration() * 2,
	}
}

// Start runs the configured number of polling workers until application shutdown.
func (w *VideoTranscoder) Start(ctx context.Context) error {
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	w.mu.Lock()
	w.cancel = cancel
	w.done = done
	w.mu.Unlock()
	defer close(done)

	w.recoverClaims(workerCtx)
	var group sync.WaitGroup
	for index := 0; index < w.workers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			w.runWorker(workerCtx)
		}()
	}
	recoveryInterval := min(w.staleAfter/2, time.Minute)
	if recoveryInterval <= 0 {
		recoveryInterval = time.Minute
	}
	ticker := time.NewTicker(recoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-workerCtx.Done():
			group.Wait()
			return nil
		case <-ticker.C:
			w.recoverClaims(workerCtx)
		}
	}
}

// Stop cancels workers and waits for all claimed FFmpeg processes to exit.
func (w *VideoTranscoder) Stop(ctx context.Context) error {
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

func (w *VideoTranscoder) runWorker(ctx context.Context) {
	for {
		processed, err := w.videoUsecase.ProcessNextVideoUpload(ctx)
		if err != nil {
			log.Error("process queued video upload", "error", err)
		}
		if processed && err == nil {
			continue
		}
		timer := time.NewTimer(w.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (w *VideoTranscoder) recoverClaims(ctx context.Context) {
	recovered, err := w.videoUsecase.RecoverStaleVideoUploads(ctx, time.Now().Add(-w.staleAfter))
	if err != nil {
		log.Error("recover stale video transcodes", "error", err)
		return
	}
	if recovered > 0 {
		log.Info("recovered stale video transcodes", "count", recovered)
	}
}
