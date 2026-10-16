package media

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreAndRemoveAvatar(t *testing.T) {
	t.Parallel()
	manager := newTestManager(t, 1024)
	if err := os.MkdirAll(manager.AvatarRoot(), 0o755); err != nil {
		t.Fatal(err)
	}
	pngBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	url, err := manager.StoreAvatar(context.Background(), pngBytes)
	if err != nil {
		t.Fatalf("StoreAvatar() error = %v", err)
	}
	name := filepath.Base(url)
	if _, err := os.Stat(filepath.Join(manager.AvatarRoot(), name)); err != nil {
		t.Fatalf("stored avatar: %v", err)
	}
	if err := manager.RemoveAvatar(url); err != nil {
		t.Fatalf("RemoveAvatar() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(manager.AvatarRoot(), name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed avatar stat error = %v", err)
	}
}

func TestStoreAvatarRejectsUnsupportedContent(t *testing.T) {
	t.Parallel()
	manager := newTestManager(t, 1024)
	if err := os.MkdirAll(manager.AvatarRoot(), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.StoreAvatar(context.Background(), []byte("not an image")); !errors.Is(err, ErrAvatarUnsupported) {
		t.Fatalf("StoreAvatar() error = %v, want %v", err, ErrAvatarUnsupported)
	}
}

func TestStoreAvatarRejectsOversizedBody(t *testing.T) {
	t.Parallel()
	manager := newTestManager(t, 4)
	if _, err := manager.StoreAvatar(context.Background(), make([]byte, 1025)); !errors.Is(err, ErrAvatarTooLarge) {
		t.Fatalf("StoreAvatar() error = %v, want %v", err, ErrAvatarTooLarge)
	}
}
