package media

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
)

var (
	// ErrAvatarTooLarge reports an avatar stream that exceeds the configured image limit.
	ErrAvatarTooLarge = errors.New("avatar too large")
	// ErrAvatarUnsupported reports an image that is not a valid JPEG or PNG.
	ErrAvatarUnsupported = errors.New("avatar format is unsupported")
)

const avatarURLPrefix = "/media/avatars/"

// StoreAvatar validates and atomically publishes one buffered JPEG or PNG avatar.
func (m *Manager) StoreAvatar(ctx context.Context, data []byte) (string, error) {
	if len(data) == 0 {
		return "", ErrAvatarUnsupported
	}
	if int64(len(data)) > m.maxCoverBytes {
		return "", ErrAvatarTooLarge
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	temp, err := os.CreateTemp(m.AvatarRoot(), ".avatar-*.part")
	if err != nil {
		return "", err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	_, writeErr := temp.Write(data)
	syncErr := temp.Sync()
	closeErr := temp.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if syncErr != nil {
		return "", syncErr
	}
	if closeErr != nil {
		return "", closeErr
	}

	file, err := os.Open(tempPath)
	if err != nil {
		return "", err
	}
	config, format, decodeErr := image.DecodeConfig(file)
	file.Close()
	if decodeErr != nil || (format != "jpeg" && format != "png") || config.Width <= 0 || config.Height <= 0 || config.Width > 4096 || config.Height > 4096 {
		return "", ErrAvatarUnsupported
	}

	id, err := newJobID()
	if err != nil {
		return "", err
	}
	extension := ".jpg"
	if format == "png" {
		extension = ".png"
	}
	name := id + extension
	if err := os.Chmod(tempPath, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tempPath, filepath.Join(m.AvatarRoot(), name)); err != nil {
		return "", err
	}
	return avatarURLPrefix + name, nil
}

// RemoveAvatar deletes only files previously published under the managed avatar URL prefix.
func (m *Manager) RemoveAvatar(avatarURL string) error {
	if avatarURL == "" || !strings.HasPrefix(avatarURL, avatarURLPrefix) {
		return nil
	}
	name := strings.TrimPrefix(avatarURL, avatarURLPrefix)
	if name == "" || filepath.Base(name) != name {
		return fmt.Errorf("invalid managed avatar URL")
	}
	err := os.Remove(filepath.Join(m.AvatarRoot(), name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
