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
