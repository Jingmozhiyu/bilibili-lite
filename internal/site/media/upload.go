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
	"regexp"
	"time"
)

const uploadHeartbeatName = ".heartbeat"

var uploadJobIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// ErrUploadTooLarge reports that an upload exceeded the configured byte limit.
var ErrUploadTooLarge = errors.New("upload too large")

// ErrCoverTooLarge reports that a custom cover exceeded the configured byte limit.
var ErrCoverTooLarge = errors.New("cover too large")

// UploadJob identifies the private files used by one in-progress upload.
type UploadJob struct {
	id         string
	directory  string
	sourcePath string
	coverPath  string
	outputDir  string
}

// CreateUploadJob allocates an unpredictable private directory for an upload and its DASH output.
func (m *Manager) CreateUploadJob() (*UploadJob, error) {
	jobID, err := newJobID()
	if err != nil {
		return nil, err
	}
	job := &UploadJob{
		id:        jobID,
		directory: filepath.Join(m.uploadRoot(), jobID),
	}
	job.sourcePath = filepath.Join(job.directory, "source.mp4.part")
	job.coverPath = filepath.Join(job.directory, "cover.part")
	job.outputDir = filepath.Join(job.directory, "output")
	if err := os.MkdirAll(job.outputDir, 0o755); err != nil {
		return nil, err
	}
	if err := job.touchHeartbeat(); err != nil {
		return nil, err
	}
	return job, nil
}

// OpenUploadJob restores a persisted upload job for background processing.
func (m *Manager) OpenUploadJob(jobID string) (*UploadJob, error) {
	if !uploadJobIDPattern.MatchString(jobID) {
		return nil, fmt.Errorf("invalid upload job id")
	}
	directory := filepath.Join(m.uploadRoot(), jobID)
	job := &UploadJob{
		id: jobID, directory: directory,
		sourcePath: filepath.Join(directory, "source.mp4.part"),
		coverPath:  filepath.Join(directory, "cover.part"),
		outputDir:  filepath.Join(directory, "output"),
	}
	if _, err := os.Stat(job.sourcePath); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(job.outputDir, 0o755); err != nil {
		return nil, err
	}
	if err := job.touchHeartbeat(); err != nil {
		return nil, err
	}
	return job, nil
}

// ID returns the opaque identifier persisted with a queued upload.
func (j *UploadJob) ID() string {
	if j == nil {
		return ""
	}
	return j.id
}

// HasCover reports whether the upload includes a custom cover file.
func (j *UploadJob) HasCover() bool {
	if j == nil {
		return false
	}
	_, err := os.Stat(j.coverPath)
	return err == nil
}

// StoreUpload streams the HTTP-backed reader into the job while enforcing context and size limits.
func (m *Manager) StoreUpload(ctx context.Context, job *UploadJob, source io.Reader) (int64, error) {
	if job == nil || source == nil {
		return 0, fmt.Errorf("upload job and source are required")
	}
	return storeReader(ctx, job, job.sourcePath, source, m.maxUploadBytes, ErrUploadTooLarge)
}

// StoreCover writes a bounded optional custom cover into the private upload job.
func (m *Manager) StoreCover(ctx context.Context, job *UploadJob, source io.Reader) error {
	if job == nil || source == nil {
		return fmt.Errorf("upload job and cover source are required")
	}
	_, err := storeReader(ctx, job, job.coverPath, source, m.maxCoverBytes, ErrCoverTooLarge)
	return err
}

// RemoveVideo deletes all publicly served media belonging to a BV identifier.
func (m *Manager) RemoveVideo(bvid string) error {
	if bvid == "" {
		return nil
	}
	return os.RemoveAll(filepath.Join(m.DASHRoot(), bvid))
}

func storeReader(ctx context.Context, job *UploadJob, path string, source io.Reader, limit int64, limitError error) (int64, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	buffer := make([]byte, 1024*1024)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		n, readErr := source.Read(buffer)
		if n > 0 {
			written += int64(n)
			if written > limit {
				return written, limitError
			}
			if _, err := file.Write(buffer[:n]); err != nil {
				return written, err
			}
			if err := job.touchHeartbeat(); err != nil {
				return written, err
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return written, file.Sync()
			}
			return written, readErr
		}
	}
}

// PublishDASH atomically moves a completed job into its public BV directory.
func (m *Manager) PublishDASH(job *UploadJob, bvid string) (manifestURL string, publishedDir string, err error) {
	if job == nil || bvid == "" {
		return "", "", fmt.Errorf("upload job and BV id are required")
	}
	finalDir := filepath.Join(m.DASHRoot(), bvid)
	// A process can stop after moving a complete manifest but before marking its
	// database row ready. The next claim owns the same processing-only BVID, so
	// replacing that unpublished directory makes publication crash-retryable.
	if err := os.RemoveAll(finalDir); err != nil {
		return "", "", fmt.Errorf("replace previous DASH directory for %s: %w", bvid, err)
	}
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
func (m *Manager) CleanupStaleUploads(activeJobIDs map[string]struct{}) (int, error) {
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
		if _, active := activeJobIDs[entry.Name()]; active {
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
