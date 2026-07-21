package media

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"bilibili-lite/internal/conf"

	"github.com/google/wire"
)

// ProviderSet is media infrastructure providers.
var ProviderSet = wire.NewSet(NewManager)

// Manager owns local upload limits, temporary jobs, and published DASH files.
type Manager struct {
	root              string
	uploadIdleTimeout time.Duration
	transcodeTimeout  time.Duration
	maxUploadBytes    int64
	maxCoverBytes     int64
}

// NewManager validates media configuration and prepares local upload and DASH directories.
func NewManager(dataConfig *conf.Data) (*Manager, error) {
	if dataConfig == nil || dataConfig.Media == nil || dataConfig.Media.StorageDir == "" {
		return nil, fmt.Errorf("media storage configuration is required")
	}
	uploadIdleTimeout := dataConfig.Media.UploadIdleTimeout.AsDuration()
	transcodeTimeout := dataConfig.Media.TranscodeTimeout.AsDuration()
	if uploadIdleTimeout <= 0 || transcodeTimeout <= 0 || dataConfig.Media.MaxUploadBytes <= 0 || dataConfig.Media.MaxCoverBytes <= 0 {
		return nil, fmt.Errorf("media timeout and size limits must be positive")
	}
	root, err := filepath.Abs(dataConfig.Media.StorageDir)
	if err != nil {
		return nil, fmt.Errorf("resolve media storage path: %w", err)
	}
	manager := newManager(root, uploadIdleTimeout, transcodeTimeout, dataConfig.Media.MaxUploadBytes, dataConfig.Media.MaxCoverBytes)
	for _, dir := range []string{manager.root, manager.uploadRoot(), manager.DASHRoot()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create media storage %s: %w", dir, err)
		}
	}
	return manager, nil
}

// DASHRoot returns the absolute directory exposed by the HTTP media file server.
func (m *Manager) DASHRoot() string {
	return filepath.Join(m.root, "dash")
}

func newManager(root string, uploadIdleTimeout, transcodeTimeout time.Duration, maxUploadBytes, maxCoverBytes int64) *Manager {
	return &Manager{
		root:              root,
		uploadIdleTimeout: uploadIdleTimeout,
		transcodeTimeout:  transcodeTimeout,
		maxUploadBytes:    maxUploadBytes,
		maxCoverBytes:     maxCoverBytes,
	}
}

func (m *Manager) uploadRoot() string {
	return filepath.Join(m.root, ".uploads")
}
