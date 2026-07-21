package media

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreUpload(t *testing.T) {
	tests := []struct {
		name      string
		content   []byte
		limit     int64
		wantError error
	}{
		{name: "copies upload", content: []byte("video data"), limit: 32},
		{name: "rejects oversized upload", content: []byte("video data"), limit: 4, wantError: ErrUploadTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := newTestManager(t, tt.limit)
			job, err := manager.CreateUploadJob()
			if err != nil {
				t.Fatalf("CreateUploadJob() error = %v", err)
			}
			err = manager.StoreUpload(context.Background(), job, bytes.NewReader(tt.content))
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("StoreUpload() error = %v, want %v", err, tt.wantError)
			}
			if tt.wantError != nil {
				return
			}
			got, err := os.ReadFile(job.sourcePath)
			if err != nil {
				t.Fatalf("read copied upload: %v", err)
			}
			if !bytes.Equal(got, tt.content) {
				t.Fatalf("copied content = %q, want %q", got, tt.content)
			}
		})
	}
}

func TestCleanupStaleUploads(t *testing.T) {
	manager := newTestManager(t, 32)
	staleJob, err := manager.CreateUploadJob()
	if err != nil {
		t.Fatalf("create stale job: %v", err)
	}
	activeJob, err := manager.CreateUploadJob()
	if err != nil {
		t.Fatalf("create active job: %v", err)
	}
	old := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(filepath.Join(staleJob.directory, uploadHeartbeatName), old, old); err != nil {
		t.Fatalf("age stale heartbeat: %v", err)
	}

	removed, err := manager.CleanupStaleUploads()
	if err != nil {
		t.Fatalf("CleanupStaleUploads() error = %v", err)
	}
	if removed != 1 {
		t.Fatalf("CleanupStaleUploads() removed = %d, want 1", removed)
	}
	if _, err := os.Stat(staleJob.directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale upload still exists: %v", err)
	}
	if _, err := os.Stat(activeJob.directory); err != nil {
		t.Fatalf("active upload was removed: %v", err)
	}
}

func newTestManager(t *testing.T, maxUploadBytes int64) *Manager {
	t.Helper()
	root := t.TempDir()
	manager := newManager(root, time.Minute, time.Minute, maxUploadBytes, 1024)
	for _, dir := range []string{manager.uploadRoot(), manager.DASHRoot()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create media directory: %v", err)
		}
	}
	return manager
}
