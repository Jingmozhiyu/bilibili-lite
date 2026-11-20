package media

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"bilibili-lite/internal/conf"

	"google.golang.org/protobuf/types/known/durationpb"
)

func TestNewManagerCreatesPersistentLayout(t *testing.T) {
	root := filepath.Join(t.TempDir(), "media")
	manager, err := NewManager(&conf.Data{Media: &conf.Data_Media{
		StorageDir: root, UploadIdleTimeout: durationpb.New(time.Minute), TranscodeTimeout: durationpb.New(time.Minute),
		MaxUploadBytes: 1024, MaxCoverBytes: 512, MaxUserStorageBytes: 4096,
		TranscodeWorkers: 1, TranscodePollInterval: durationpb.New(time.Second),
	}})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	for _, directory := range []string{manager.DASHRoot(), manager.AvatarRoot(), manager.uploadRoot()} {
		info, err := os.Stat(directory)
		if err != nil || !info.IsDir() {
			t.Fatalf("persistent media directory %q was not created: %v", directory, err)
		}
	}
}

func TestPublishDASHReplacesInterruptedPublication(t *testing.T) {
	manager := newManager(t.TempDir(), time.Minute, time.Minute, 1024, 1024)
	for _, directory := range []string{manager.uploadRoot(), manager.DASHRoot()} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	job, err := manager.CreateUploadJob()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(job.outputDir, "manifest.mpd"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldDir := filepath.Join(manager.DASHRoot(), "BV1")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "manifest.mpd"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	manifestURL, _, err := manager.PublishDASH(job, "BV1")
	if err != nil {
		t.Fatal(err)
	}
	if manifestURL != "/media/dash/BV1/manifest.mpd" {
		t.Fatalf("manifest URL = %q", manifestURL)
	}
	content, err := os.ReadFile(filepath.Join(oldDir, "manifest.mpd"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "new" {
		t.Fatalf("published manifest = %q, want new", content)
	}
}
