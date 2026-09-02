package assetstore

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	ModeLocal = "local"
	ModeS3    = "s3"

	DefaultMaxBytes       int64 = 256 << 20
	MaxAllowedBytes       int64 = 2 << 30
	DefaultPresignSeconds       = 7 * 24 * 60 * 60
	MaxPresignSeconds           = 7 * 24 * 60 * 60
)

var (
	ErrDisabled       = errors.New("asset storage is not configured")
	ErrInvalidObject  = errors.New("invalid asset object key")
	allowedContentMap = map[string]struct{}{
		"image/jpeg": {}, "image/png": {}, "image/webp": {}, "image/gif": {},
		"video/mp4": {}, "video/webm": {}, "video/quicktime": {},
		"audio/mpeg": {}, "audio/mp4": {}, "audio/wav": {}, "audio/x-wav": {}, "audio/ogg": {},
	}
)

type Config struct {
	Mode           string
	LocalDir       string
	PublicURL      string
	S3Endpoint     string
	S3Bucket       string
	S3Region       string
	S3AccessKey    string
	S3SecretKey    string
	S3Prefix       string
	PresignSeconds int
	MaxBytes       int64
}

type Object struct {
	Key          string `json:"key"`
	URL          string `json:"url"`
	Backend      string `json:"backend"`
	OriginalName string `json:"original_name"`
	ContentType  string `json:"content_type"`
	Size         int64  `json:"size"`
}

type Store struct {
	config Config
	client *minio.Client
}

func LoadConfig() Config {
	maxMB := common.GetEnvOrDefault("ASSET_STORAGE_MAX_MB", int(DefaultMaxBytes/(1<<20)))
	if maxMB < 1 {
		maxMB = int(DefaultMaxBytes / (1 << 20))
	}
	return Config{
		Mode:           strings.ToLower(strings.TrimSpace(common.GetEnvOrDefaultString("ASSET_STORAGE_MODE", ModeLocal))),
		LocalDir:       common.GetEnvOrDefaultString("ASSET_STORAGE_DIR", filepath.Join("data", "assets")),
		PublicURL:      strings.TrimRight(strings.TrimSpace(common.GetEnvOrDefaultString("ASSET_STORAGE_PUBLIC_URL", "")), "/"),
		S3Endpoint:     strings.TrimSpace(common.GetEnvOrDefaultString("ASSET_STORAGE_S3_ENDPOINT", "")),
		S3Bucket:       strings.TrimSpace(common.GetEnvOrDefaultString("ASSET_STORAGE_S3_BUCKET", "")),
		S3Region:       strings.TrimSpace(common.GetEnvOrDefaultString("ASSET_STORAGE_S3_REGION", "us-east-1")),
		S3AccessKey:    common.GetEnvOrDefaultString("ASSET_STORAGE_S3_ACCESS_KEY", ""),
		S3SecretKey:    common.GetEnvOrDefaultString("ASSET_STORAGE_S3_SECRET_KEY", ""),
		S3Prefix:       strings.Trim(strings.TrimSpace(common.GetEnvOrDefaultString("ASSET_STORAGE_S3_PREFIX", "assets")), "/"),
		PresignSeconds: common.GetEnvOrDefault("ASSET_STORAGE_URL_TTL_SECONDS", DefaultPresignSeconds),
		MaxBytes:       int64(maxMB) * (1 << 20),
	}
}

func ValidateConfig(config Config) error {
	if config.Mode != ModeLocal && config.Mode != ModeS3 {
		return fmt.Errorf("unsupported asset storage mode %q", config.Mode)
	}
	if config.MaxBytes <= 0 || config.MaxBytes > MaxAllowedBytes {
		return fmt.Errorf("asset max size must be between 1 and %d bytes", MaxAllowedBytes)
	}
	if config.PresignSeconds <= 0 || config.PresignSeconds > MaxPresignSeconds {
		return fmt.Errorf("asset URL TTL must be between 1 and %d seconds", MaxPresignSeconds)
	}
	if config.Mode == ModeLocal {
		if strings.TrimSpace(config.LocalDir) == "" {
			return errors.New("asset storage directory is required")
		}
		return nil
	}
	if config.S3Endpoint == "" || config.S3Bucket == "" || config.S3AccessKey == "" || config.S3SecretKey == "" {
		return errors.New("S3 endpoint, bucket, access key, and secret key are required")
	}
	endpoint, err := url.Parse(config.S3Endpoint)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.User != nil || endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return errors.New("S3 endpoint must be an absolute HTTP(S) URL without credentials or query")
	}
	if strings.TrimSpace(config.S3Bucket) != config.S3Bucket || strings.Contains(config.S3Bucket, "/") {
		return errors.New("S3 bucket is invalid")
	}
	return nil
}

func New(config Config) (*Store, error) {
	if err := ValidateConfig(config); err != nil {
		return nil, err
	}
	store := &Store{config: config}
	if config.Mode == ModeLocal {
		if err := os.MkdirAll(config.LocalDir, 0o750); err != nil {
			return nil, fmt.Errorf("create asset storage directory: %w", err)
		}
		return store, nil
	}
	endpoint, _ := url.Parse(config.S3Endpoint)
	client, err := minio.New(endpoint.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(config.S3AccessKey, config.S3SecretKey, ""),
		Secure: endpoint.Scheme == "https",
		Region: config.S3Region,
	})
	if err != nil {
		return nil, fmt.Errorf("create S3 client: %w", err)
	}
	store.client = client
	return store, nil
}

func (s *Store) Config() Config { return s.config }

func (s *Store) Upload(ctx context.Context, reader io.Reader, originalName, contentType string, size int64) (Object, error) {
	if s == nil {
		return Object{}, ErrDisabled
	}
	if reader == nil || size < 0 || size > s.config.MaxBytes {
		return Object{}, fmt.Errorf("asset exceeds configured size limit of %d bytes", s.config.MaxBytes)
	}
	contentType = NormalizeContentType(contentType)
	if !IsAllowedContentType(contentType) {
		return Object{}, fmt.Errorf("unsupported asset content type %q", contentType)
	}
	key, err := randomObjectKey(contentType)
	if err != nil {
		return Object{}, err
	}
	if s.config.Mode == ModeLocal {
		path := filepath.Join(s.config.LocalDir, key)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
		if err != nil {
			return Object{}, fmt.Errorf("create asset object: %w", err)
		}
		written, copyErr := io.Copy(file, io.LimitReader(reader, s.config.MaxBytes+1))
		closeErr := file.Close()
		if copyErr != nil {
			_ = os.Remove(path)
			return Object{}, fmt.Errorf("write asset object: %w", copyErr)
		}
		if closeErr != nil || written > s.config.MaxBytes || (size > 0 && written != size) {
			_ = os.Remove(path)
			return Object{}, fmt.Errorf("asset size exceeds configured limit of %d bytes", s.config.MaxBytes)
		}
		return Object{Key: key, Backend: ModeLocal, OriginalName: originalName, ContentType: contentType, Size: written}, nil
	}

	objectKey := key
	if s.config.S3Prefix != "" {
		objectKey = s.config.S3Prefix + "/" + key
	}
	if _, err = s.client.PutObject(ctx, s.config.S3Bucket, objectKey, io.LimitReader(reader, s.config.MaxBytes+1), size, minio.PutObjectOptions{ContentType: contentType}); err != nil {
		return Object{}, fmt.Errorf("upload asset to S3: %w", err)
	}
	assetURL, err := s.client.PresignedGetObject(ctx, s.config.S3Bucket, objectKey, time.Duration(s.config.PresignSeconds)*time.Second, nil)
	if err != nil {
		return Object{}, fmt.Errorf("create S3 asset URL: %w", err)
	}
	return Object{Key: key, URL: assetURL.String(), Backend: ModeS3, OriginalName: originalName, ContentType: contentType, Size: size}, nil
}

func (s *Store) Delete(ctx context.Context, key string) error {
	if s == nil {
		return ErrDisabled
	}
	if err := validateKey(key); err != nil {
		return err
	}
	if s.config.Mode == ModeLocal {
		if err := os.Remove(filepath.Join(s.config.LocalDir, key)); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	objectKey := key
	if s.config.S3Prefix != "" {
		objectKey = s.config.S3Prefix + "/" + key
	}
	return s.client.RemoveObject(ctx, s.config.S3Bucket, objectKey, minio.RemoveObjectOptions{})
}

func (s *Store) LocalPath(key string) (string, error) {
	if s == nil || s.config.Mode != ModeLocal {
		return "", ErrDisabled
	}
	if err := validateKey(key); err != nil {
		return "", err
	}
	return filepath.Join(s.config.LocalDir, key), nil
}

func (s *Store) URL(key, fallbackBase string) (string, error) {
	if s == nil {
		return "", ErrDisabled
	}
	if err := validateKey(key); err != nil {
		return "", err
	}
	if s.config.Mode == ModeS3 {
		objectKey := key
		if s.config.S3Prefix != "" {
			objectKey = s.config.S3Prefix + "/" + key
		}
		value, err := s.client.PresignedGetObject(context.Background(), s.config.S3Bucket, objectKey, time.Duration(s.config.PresignSeconds)*time.Second, nil)
		if err != nil {
			return "", err
		}
		return value.String(), nil
	}
	base := strings.TrimRight(s.config.PublicURL, "/")
	if base == "" {
		base = strings.TrimRight(fallbackBase, "/")
	}
	if base == "" {
		return "", errors.New("asset public URL is not configured")
	}
	return base + "/api/mobilecloud/uploads/" + url.PathEscape(key), nil
}

func NormalizeContentType(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.Split(value, ";")[0]))
}

func IsAllowedContentType(value string) bool {
	_, ok := allowedContentMap[NormalizeContentType(value)]
	return ok
}

func ContentTypeExtension(value string) string {
	switch NormalizeContentType(value) {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "video/quicktime":
		return ".mov"
	case "audio/mpeg":
		return ".mp3"
	case "audio/mp4":
		return ".m4a"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/ogg":
		return ".ogg"
	default:
		ext := mime.TypeByExtension(filepath.Ext(value))
		if ext != "" {
			return ext
		}
		return ".bin"
	}
}

func randomObjectKey(contentType string) (string, error) {
	var bytes [24]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generate asset object key: %w", err)
	}
	return fmt.Sprintf("%x%s", bytes, ContentTypeExtension(contentType)), nil
}

func validateKey(key string) error {
	if key == "" || filepath.Base(key) != key || strings.ContainsAny(key, `/\\`) || key == "." || key == ".." {
		return ErrInvalidObject
	}
	return nil
}

// Init initializes the process-wide asset store. A local store is enabled by
// default; invalid optional S3 settings are logged and leave uploads disabled.
var global *Store

func Init() error {
	config := LoadConfig()
	store, err := New(config)
	if err != nil {
		common.SysError("asset storage disabled: " + err.Error())
		global = nil
		return err
	}
	global = store
	common.SysLog(fmt.Sprintf("asset storage initialized in %s mode (max %d bytes)", config.Mode, config.MaxBytes))
	return nil
}

func Get() *Store { return global }
