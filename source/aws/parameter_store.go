package aws

import (
	"context"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/yacchi/jubako/source"
	"github.com/yacchi/jubako/watcher"
)

// ParameterStoreSource loads configuration from an AWS Systems Manager Parameter Store parameter.
// This source is read-only; Save operations return ErrSaveNotSupported.
// Change detection uses parameter version for efficient polling.
//
// Note: ParameterStoreSource does not implement its own synchronization. When used with
// layer.New(), the Layer provides operation-level synchronization between
// Load, Save, and poll operations.
type ParameterStoreSource struct {
	name        string
	withDecrypt bool
	cfg         clientConfig
	client      *ssm.Client

	clientInit    sync.Once
	clientInitErr error
}

// Ensure ParameterStoreSource implements the source.Source interface.
var _ source.Source = (*ParameterStoreSource)(nil)

// Ensure ParameterStoreSource implements the source.WatchableSource interface.
var _ source.WatchableSource = (*ParameterStoreSource)(nil)

// TypeParameterStore is the source type identifier for SSM Parameter Store sources.
const TypeParameterStore source.SourceType = "parameter-store"

// ParameterStoreOption configures a ParameterStoreSource.
// It implements the Option interface.
type ParameterStoreOption func(*ParameterStoreSource)

// awsSourceOption implements the Option interface.
func (ParameterStoreOption) awsSourceOption() {}

// WithParameterStoreClient sets a custom SSM client for Parameter Store.
// This overrides WithAWSConfig for the SSM client.
func WithParameterStoreClient(client *ssm.Client) ParameterStoreOption {
	return func(s *ParameterStoreSource) {
		s.client = client
	}
}

// WithDecryption enables decryption for SecureString parameters.
// Default is false.
func WithDecryption(decrypt bool) ParameterStoreOption {
	return func(s *ParameterStoreSource) {
		s.withDecrypt = decrypt
	}
}

// NewParameterStoreSource creates an SSM Parameter Store source for the given parameter name.
//
// Example:
//
//	src := aws.NewParameterStoreSource("/app/config")
//	src := aws.NewParameterStoreSource("/app/secrets", aws.WithDecryption(true))
//	src := aws.NewParameterStoreSource("/app/config", aws.WithAWSConfig(cfg))
//	src := aws.NewParameterStoreSource("/app/config", aws.WithParameterStoreClient(customClient))
func NewParameterStoreSource(name string, opts ...Option) *ParameterStoreSource {
	s := &ParameterStoreSource{
		name: name,
	}

	for _, opt := range opts {
		switch o := opt.(type) {
		case ClientOption:
			o(&s.cfg)
		case ParameterStoreOption:
			o(s)
		}
	}

	return s
}

// ensureClient creates a default SSM client if one was not provided.
func (s *ParameterStoreSource) ensureClient(ctx context.Context) error {
	if s.client != nil {
		return nil
	}

	s.clientInit.Do(func() {
		cfg, err := loadAWSConfig(ctx, &s.cfg)
		if err != nil {
			s.clientInitErr = err
			return
		}
		s.client = ssm.NewFromConfig(cfg)
	})
	return s.clientInitErr
}

func (s *ParameterStoreSource) getParameter(ctx context.Context) (version int64, value []byte, err error) {
	if err := ctx.Err(); err != nil {
		return 0, nil, err
	}
	if err := s.ensureClient(ctx); err != nil {
		return 0, nil, err
	}

	result, err := s.client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(s.name),
		WithDecryption: aws.Bool(s.withDecrypt),
	})
	if err != nil {
		return 0, nil, fmt.Errorf("failed to get parameter %q: %w", s.name, err)
	}

	if result.Parameter == nil {
		return 0, nil, fmt.Errorf("parameter %q not found", s.name)
	}
	if result.Parameter.Value == nil {
		return 0, nil, fmt.Errorf("parameter %q has no value", s.name)
	}

	return result.Parameter.Version, []byte(*result.Parameter.Value), nil
}

// Load implements the source.Source interface.
// Fetches the parameter from SSM and caches the version for change detection.
func (s *ParameterStoreSource) Load(ctx context.Context) ([]byte, error) {
	_, value, err := s.getParameter(ctx)
	return value, err
}

// Save implements the source.Source interface.
// Parameter Store source is read-only; this always returns ErrSaveNotSupported.
func (s *ParameterStoreSource) Save(ctx context.Context, updateFunc source.UpdateFunc) error {
	return source.ErrSaveNotSupported
}

// CanSave returns false because Parameter Store sources do not support saving.
func (s *ParameterStoreSource) CanSave() bool {
	return false
}

// Type returns the source type identifier.
func (s *ParameterStoreSource) Type() source.SourceType {
	return TypeParameterStore
}

// Name returns the SSM parameter name.
func (s *ParameterStoreSource) Name() string {
	return s.name
}

// Watch implements the source.WatchableSource interface.
// Returns a WatcherInitializer that creates a PollingWatcher using version-based
// change detection.
func (s *ParameterStoreSource) Watch() (watcher.WatcherInitializer, error) {
	var lastVersion int64
	var hasVersion bool

	pollOnce := func(ctx context.Context) (bool, []byte, error) {
		version, value, err := s.getParameter(ctx)
		if err != nil {
			return false, nil, err
		}

		if hasVersion && version == lastVersion {
			return false, nil, nil
		}
		lastVersion = version
		hasVersion = true

		return true, value, nil
	}

	return watcher.NewPolling(pollOnce), nil
}
