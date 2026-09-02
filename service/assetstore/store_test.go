package assetstore

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalStoreUploadsAndDeletesObject(t *testing.T) {
	dir := t.TempDir()
	store, err := New(Config{
		Mode:           ModeLocal,
		LocalDir:       dir,
		PresignSeconds: 60,
		MaxBytes:       1024,
	})
	require.NoError(t, err)

	object, err := store.Upload(context.Background(), bytes.NewReader([]byte("video")), "clip.mp4", "video/mp4", 5)
	require.NoError(t, err)
	require.Equal(t, ModeLocal, object.Backend)
	require.NotEmpty(t, object.Key)
	require.Equal(t, int64(5), object.Size)
	require.FileExists(t, filepath.Join(dir, object.Key))

	url, err := store.URL(object.Key, "https://gateway.example")
	require.NoError(t, err)
	require.Contains(t, url, "/api/mobilecloud/uploads/")
	require.NoError(t, store.Delete(context.Background(), object.Key))
	require.NoFileExists(t, filepath.Join(dir, object.Key))
}

func TestLocalStoreRejectsUnsupportedAndOversizedObjects(t *testing.T) {
	store, err := New(Config{Mode: ModeLocal, LocalDir: t.TempDir(), PresignSeconds: 60, MaxBytes: 4})
	require.NoError(t, err)
	_, err = store.Upload(context.Background(), bytes.NewReader([]byte("x")), "x.txt", "text/plain", 1)
	require.Error(t, err)
	_, err = store.Upload(context.Background(), bytes.NewReader([]byte("12345")), "x.mp4", "video/mp4", 5)
	require.Error(t, err)
}

func TestLocalStoreRejectsTraversal(t *testing.T) {
	store, err := New(Config{Mode: ModeLocal, LocalDir: t.TempDir(), PresignSeconds: 60, MaxBytes: 4})
	require.NoError(t, err)
	_, err = store.LocalPath("../secret")
	require.ErrorIs(t, err, ErrInvalidObject)
	require.Error(t, store.Delete(context.Background(), "../secret"))
}

func TestValidateConfigRequiresS3Credentials(t *testing.T) {
	err := ValidateConfig(Config{Mode: ModeS3, S3Endpoint: "https://s3.example", S3Bucket: "assets", PresignSeconds: 60, MaxBytes: 10})
	require.Error(t, err)
}
