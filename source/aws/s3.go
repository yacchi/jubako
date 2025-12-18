package aws

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/yacchi/jubako/source"
	"github.com/yacchi/jubako/watcher"
)

// S3Source loads configuration from an S3 object.
// This source is read-only; Save operations return ErrSaveNotSupported.
// Change detection uses ETags for efficient polling.
//
// Note: S3Source does not implement its own synchronization. When used with
// layer.New(), the Layer provides operation-level synchronization between
// Load, Save, and poll operations.
type S3Source struct {
	bucket string
	key    string
	cfg    clientConfig
	client *s3.Client

	clientInit    sync.Once
	clientInitErr error
}

// Ensure S3Source implements the source.Source interface.
var _ source.Source = (*S3Source)(nil)

// Ensure S3Source implements the source.WatchableSource interface.
var _ source.WatchableSource = (*S3Source)(nil)

// Ensure S3Source implements the source.NotExistCapable interface.
var _ source.NotExistCapable = (*S3Source)(nil)

// TypeS3 is the source type identifier for S3 sources.
const TypeS3 source.SourceType = "s3"

// S3Option configures an S3Source.
// It implements the Option interface.
type S3Option func(*S3Source)

// awsSourceOption implements the Option interface.
func (S3Option) awsSourceOption() {}

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
func NewS3Source(bucket, key string, opts ...Option) *S3Source {
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

	s.clientInit.Do(func() {
		cfg, err := loadAWSConfig(ctx, &s.cfg)
		if err != nil {
			s.clientInitErr = err
			return
		}
		s.client = s3.NewFromConfig(cfg)
	})
	return s.clientInitErr
}

// Load implements the source.Source interface.
// Fetches the object from S3 and caches the ETag for change detection.
func (s *S3Source) Load(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	data, _, err := s.fetchObject(ctx, nil)
	return data, err
}

// fetchObject fetches the object from S3, optionally with conditional GET.
// If ifNoneMatch is provided, returns (nil, nil) when the object hasn't changed.
func (s *S3Source) fetchObject(ctx context.Context, ifNoneMatch *string) ([]byte, *string, error) {
	if err := s.ensureClient(ctx); err != nil {
		return nil, nil, err
	}

	input := &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key),
	}
	if ifNoneMatch != nil {
		input.IfNoneMatch = ifNoneMatch
	}

	result, err := s.client.GetObject(ctx, input)
	if err != nil {
		// Check for 304 Not Modified
		var respErr *awshttp.ResponseError
		if errors.As(err, &respErr) {
			switch respErr.HTTPStatusCode() {
			case http.StatusNotModified:
				return nil, ifNoneMatch, nil
			case http.StatusNotFound:
				// Object does not exist
				return nil, nil, source.NewNotExistError(
					fmt.Sprintf("s3://%s/%s", s.bucket, s.key), err)
			}
		}
		return nil, nil, fmt.Errorf("failed to get object s3://%s/%s: %w", s.bucket, s.key, err)
	}
	defer result.Body.Close()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read object body: %w", err)
	}

	return data, result.ETag, nil
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

// CanNotExist returns true because S3 objects can be missing.
func (s *S3Source) CanNotExist() bool {
	return true
}

// Type returns the source type identifier.
func (s *S3Source) Type() source.SourceType {
	return TypeS3
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
// Returns a WatcherInitializer that creates a PollingWatcher using ETag-based
// change detection.
func (s *S3Source) Watch() (watcher.WatcherInitializer, error) {
	var lastETag *string
	pollOnce := func(ctx context.Context) (bool, []byte, error) {
		if err := ctx.Err(); err != nil {
			return false, nil, err
		}
		data, etag, err := s.fetchObject(ctx, lastETag)
		if err != nil {
			return false, nil, err
		}
		if data == nil {
			// Not modified (304)
			return false, nil, nil
		}

		lastETag = etag
		return true, data, nil
	}

	return watcher.NewPolling(pollOnce), nil
}
