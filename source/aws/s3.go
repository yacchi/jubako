package aws

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/yacchi/jubako/source"
	"github.com/yacchi/jubako/watcher"
)

// S3Source loads configuration from an S3 object.
// This source is read-only; Save operations return ErrSaveNotSupported.
// Change detection uses ETags for efficient polling.
type S3Source struct {
	bucket string
	key    string
	cfg    clientConfig
	client *s3.Client

	mu      sync.Mutex
	lastTag string // cached ETag for change detection
}

// Ensure S3Source implements the source.Source interface.
var _ source.Source = (*S3Source)(nil)

// Ensure S3Source implements the source.WatchableSource interface.
var _ source.WatchableSource = (*S3Source)(nil)

// S3Option configures an S3Source.
type S3Option func(*S3Source)

// WithS3Client sets a custom S3 client.
// This overrides WithAWSConfig for the S3 client.
func WithS3Client(client *s3.Client) S3Option {
	return func(s *S3Source) {
		s.client = client
	}
}

// NewS3Source creates an S3 source for the given bucket and key.
//
// Example:
//
//	src := aws.NewS3Source("my-bucket", "config/app.yaml")
//	src := aws.NewS3Source("my-bucket", "config/app.yaml", aws.WithAWSConfig(cfg))
//	src := aws.NewS3Source("my-bucket", "config/app.yaml", aws.WithS3Client(customClient))
func NewS3Source(bucket, key string, opts ...any) *S3Source {
	s := &S3Source{
		bucket: bucket,
		key:    key,
	}

	for _, opt := range opts {
		switch o := opt.(type) {
		case ClientOption:
			o(&s.cfg)
		case S3Option:
			o(s)
		}
	}

	return s
}

// ensureClient creates a default S3 client if one was not provided.
func (s *S3Source) ensureClient(ctx context.Context) error {
	if s.client != nil {
		return nil
	}

	cfg, err := loadAWSConfig(ctx, &s.cfg)
	if err != nil {
		return err
	}

	s.client = s3.NewFromConfig(cfg)
	return nil
}

// Load implements the source.Source interface.
// Fetches the object from S3 and caches the ETag for change detection.
func (s *S3Source) Load(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := s.ensureClient(ctx); err != nil {
		return nil, err
	}

	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get object s3://%s/%s: %w", s.bucket, s.key, err)
	}
	defer result.Body.Close()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read object body: %w", err)
	}

	// Cache ETag for change detection
	s.mu.Lock()
	if result.ETag != nil {
		s.lastTag = *result.ETag
	}
	s.mu.Unlock()

	return data, nil
}

// Save implements the source.Source interface.
// S3 source is read-only; this always returns ErrSaveNotSupported.
func (s *S3Source) Save(ctx context.Context, updateFunc source.UpdateFunc) error {
	return source.ErrSaveNotSupported
}

// CanSave returns false because S3 sources do not support saving.
func (s *S3Source) CanSave() bool {
	return false
}

// Bucket returns the S3 bucket name.
func (s *S3Source) Bucket() string {
	return s.bucket
}

// Key returns the S3 object key.
func (s *S3Source) Key() string {
	return s.key
}

// Watch implements the source.WatchableSource interface.
// Returns a PollingWatcher that uses HeadObject to check for ETag changes.
func (s *S3Source) Watch() (watcher.Watcher, error) {
	return watcher.NewPolling(watcher.PollHandlerFunc(s.poll)), nil
}

// poll checks for changes using HeadObject and returns new data if changed.
func (s *S3Source) poll(ctx context.Context) ([]byte, error) {
	if err := s.ensureClient(ctx); err != nil {
		return nil, err
	}

	// Check ETag without downloading the full object
	head, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to head object s3://%s/%s: %w", s.bucket, s.key, err)
	}

	s.mu.Lock()
	currentTag := s.lastTag
	s.mu.Unlock()

	// If ETag hasn't changed, return nil to indicate no change
	if head.ETag != nil && *head.ETag == currentTag {
		return nil, nil
	}

	// ETag changed - fetch the new content
	return s.Load(ctx)
}
