package media

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const uploadHeartbeatName = ".heartbeat"

// ErrUploadTooLarge reports that an upload exceeded the configured byte limit.
var ErrUploadTooLarge = errors.New("upload too large")

// UploadJob identifies the private files used by one in-progress upload.
type UploadJob struct {
	directory  string
	sourcePath string
	outputDir  string
}

// CreateUploadJob allocates an unpredictable private directory for an upload and its DASH output.
func (m *Manager) CreateUploadJob() (*UploadJob, error) {
	jobID, err := newJobID()
	if err != nil {
		return nil, err
	}
	job := &UploadJob{
		directory: filepath.Join(m.uploadRoot(), jobID),
	}
	job.sourcePath = filepath.Join(job.directory, "source.mp4.part")
	job.outputDir = filepath.Join(job.directory, "output")
	if err := os.MkdirAll(job.outputDir, 0o755); err != nil {
		return nil, err
	}
	if err := job.touchHeartbeat(); err != nil {
		return nil, err
	}
	return job, nil
}

// StoreUpload streams the HTTP-backed reader into the job while enforcing context and size limits.
func (m *Manager) StoreUpload(ctx context.Context, job *UploadJob, source io.Reader) error {
	if job == nil || source == nil {
		return fmt.Errorf("upload job and source are required")
	}
	file, err := os.OpenFile(job.sourcePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	buffer := make([]byte, 1024*1024)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := source.Read(buffer)
		if n > 0 {
			written += int64(n)
			if written > m.maxUploadBytes {
				return ErrUploadTooLarge
			}
			if _, err := file.Write(buffer[:n]); err != nil {
				return err
			}
			if err := job.touchHeartbeat(); err != nil {
				return err
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return file.Sync()
			}
			return readErr
		}
	}
}

// PublishDASH atomically moves a completed job into its public BV directory.
func (m *Manager) PublishDASH(job *UploadJob, bvid string) (manifestURL string, publishedDir string, err error) {
	if job == nil || bvid == "" {
		return "", "", fmt.Errorf("upload job and BV id are required")
	}
	finalDir := filepath.Join(m.DASHRoot(), bvid)
	if err := os.Rename(job.outputDir, finalDir); err != nil {
		return "", "", fmt.Errorf("publish DASH directory for %s: %w", bvid, err)
	}
	return "/media/dash/" + bvid + "/manifest.mpd", finalDir, nil
}

// RemoveUploadJob deletes all temporary files belonging to a completed or failed job.
func (m *Manager) RemoveUploadJob(job *UploadJob) error {
	if job == nil {
		return nil
	}
	return os.RemoveAll(job.directory)
}

// RemovePublished deletes a DASH directory after its database transaction rolls back.
func (m *Manager) RemovePublished(path string) error {
	if path == "" {
		return nil
	}
	return os.RemoveAll(path)
}

// CleanupStaleUploads removes jobs whose heartbeat is older than the configured idle timeout.
func (m *Manager) CleanupStaleUploads() (int, error) {
	entries, err := os.ReadDir(m.uploadRoot())
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().Add(-m.uploadIdleTimeout)
	removed := 0
	var cleanupErrors []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		jobDir := filepath.Join(m.uploadRoot(), entry.Name())
		info, err := os.Stat(filepath.Join(jobDir, uploadHeartbeatName))
		if err != nil {
			info, err = entry.Info()
		}
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		if err := os.RemoveAll(jobDir); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove upload job %s: %w", entry.Name(), err))
			continue
		}
		removed++
	}
	return removed, errors.Join(cleanupErrors...)
}

func (j *UploadJob) touchHeartbeat() error {
	now := time.Now()
	path := filepath.Join(j.directory, uploadHeartbeatName)
	if err := os.Chtimes(path, now, now); err == nil {
		return nil
	}
	return os.WriteFile(path, nil, 0o600)
}

func newJobID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
