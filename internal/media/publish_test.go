package media

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
