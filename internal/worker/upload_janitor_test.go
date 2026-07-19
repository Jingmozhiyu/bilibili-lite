package worker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bilibili-lite/internal/conf"
	"bilibili-lite/internal/media"

	"google.golang.org/protobuf/types/known/durationpb"
)

func TestUploadJanitorStartAndStop(t *testing.T) {
	root := t.TempDir()
	manager, err := media.NewManager(&conf.Data{Media: &conf.Data_Media{
		StorageDir:        root,
		UploadIdleTimeout: durationpb.New(time.Minute),
		TranscodeTimeout:  durationpb.New(time.Minute),
		MaxUploadBytes:    32,
	}})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	staleDir := filepath.Join(root, ".uploads", "stale")
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatalf("create stale directory: %v", err)
	}
	heartbeat := filepath.Join(staleDir, ".heartbeat")
	if err := os.WriteFile(heartbeat, nil, 0o600); err != nil {
		t.Fatalf("create heartbeat: %v", err)
	}
	old := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(heartbeat, old, old); err != nil {
		t.Fatalf("age heartbeat: %v", err)
	}

	janitor := newUploadJanitor(manager, time.Hour)
	startResult := make(chan error, 1)
	go func() {
		startResult <- janitor.Start(context.Background())
	}()

	deadline := time.Now().Add(time.Second)
	for {
		_, err := os.Stat(staleDir)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("stale upload was not removed: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := janitor.Stop(stopCtx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if err := <-startResult; err != nil {
		t.Fatalf("Start() error = %v", err)
	}
}
