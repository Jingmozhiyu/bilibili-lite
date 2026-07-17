package data

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/go-kratos/kratos/v3/log"
)

const uploadHeartbeatName = ".heartbeat"

// startUploadJanitor periodically removes abandoned upload jobs until the data layer shuts down.
func (d *Data) startUploadJanitor(ctx context.Context) {
	d.janitorWG.Add(1)
	go func() {
		defer d.janitorWG.Done()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		d.cleanupStaleUploads()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				d.cleanupStaleUploads()
			}
		}
	}()
}

// cleanupStaleUploads deletes temporary jobs whose heartbeat has exceeded the configured idle timeout.
func (d *Data) cleanupStaleUploads() {
	root := filepath.Join(d.mediaRoot, ".uploads")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-d.uploadIdleTimeout)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		jobDir := filepath.Join(root, entry.Name())
		info, err := os.Stat(filepath.Join(jobDir, uploadHeartbeatName))
		if err != nil {
			info, err = entry.Info()
		}
		if err == nil && info.ModTime().Before(cutoff) {
			if err := os.RemoveAll(jobDir); err != nil {
				log.Error("remove stale upload", "job_id", entry.Name(), "error", err)
			} else {
				log.Info("removed stale upload", "job_id", entry.Name())
			}
		}
	}
}

// touchHeartbeat records recent upload or FFmpeg activity for stale-job detection.
func touchHeartbeat(jobDir string) error {
	now := time.Now()
	path := filepath.Join(jobDir, uploadHeartbeatName)
	if err := os.Chtimes(path, now, now); err == nil {
		return nil
	}
	return os.WriteFile(path, nil, 0o600)
}
