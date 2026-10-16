package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"bilibili-lite/internal/mall/biz"
	"bilibili-lite/internal/mall/observability"
)

// Maintenance runs one reconciliation batch at a time for the lifetime of the mall application.
type Maintenance struct {
	metrics *observability.Metrics
	u       *biz.MallUsecase
	cancel  context.CancelFunc
	done    chan struct{}
	mu      sync.Mutex
}

func NewMaintenance(u *biz.MallUsecase, metrics *observability.Metrics) *Maintenance {
	return &Maintenance{u: u, metrics: metrics}
}

func (w *Maintenance) Start(context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.done = make(chan struct{})
	go func() {
		defer close(w.done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			// A slow batch must finish before the next starts; never accumulate background goroutines.
			case <-ticker.C:
				run, stop := context.WithTimeout(ctx, 10*time.Second)
				started := time.Now()
				err := w.u.Maintain(run)
				w.metrics.MaintenanceDuration.Observe(time.Since(started).Seconds())
				if err == nil {
					w.metrics.Maintenance.WithLabelValues("success").Inc()
					w.metrics.MaintenanceSuccess.SetToCurrentTime()
				} else if ctx.Err() == nil {
					w.metrics.Maintenance.WithLabelValues("error").Inc()
				}
				stop()
				if err != nil && ctx.Err() == nil {
					slog.Error("mall maintenance failed", "error", err)
				}
			}
		}
	}()
	return nil
}

func (w *Maintenance) Stop(ctx context.Context) error {
	w.mu.Lock()
	cancel, done := w.cancel, w.done
	w.mu.Unlock()
	if cancel == nil {
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
