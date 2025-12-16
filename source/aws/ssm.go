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

// SSMSource loads configuration from an SSM Parameter Store parameter.
// This source is read-only; Save operations return ErrSaveNotSupported.
// Change detection uses parameter version for efficient polling.
type SSMSource struct {
	name        string
	withDecrypt bool
	cfg         clientConfig
	client      *ssm.Client

	mu          sync.Mutex
	lastVersion int64 // cached version for change detection
}

// Ensure SSMSource implements the source.Source interface.
var _ source.Source = (*SSMSource)(nil)

// Ensure SSMSource implements the source.WatchableSource interface.
var _ source.WatchableSource = (*SSMSource)(nil)

// SSMOption configures an SSMSource.
type SSMOption func(*SSMSource)

// WithSSMClient sets a custom SSM client.
// This overrides WithAWSConfig for the SSM client.
func WithSSMClient(client *ssm.Client) SSMOption {
	return func(s *SSMSource) {
		s.client = client
	}
}

// WithDecryption enables decryption for SecureString parameters.
// Default is false.
func WithDecryption(decrypt bool) SSMOption {
	return func(s *SSMSource) {
		s.withDecrypt = decrypt
	}
}

// NewSSMSource creates an SSM Parameter Store source for the given parameter name.
//
// Example:
//
//	src := aws.NewSSMSource("/app/config")
//	src := aws.NewSSMSource("/app/secrets", aws.WithDecryption(true))
//	src := aws.NewSSMSource("/app/config", aws.WithAWSConfig(cfg))
//	src := aws.NewSSMSource("/app/config", aws.WithSSMClient(customClient))
func NewSSMSource(name string, opts ...any) *SSMSource {
	s := &SSMSource{
		name: name,
	}

	for _, opt := range opts {
		switch o := opt.(type) {
		case ClientOption:
			o(&s.cfg)
		case SSMOption:
			o(s)
		}
	}

	return s
}

// ensureClient creates a default SSM client if one was not provided.
func (s *SSMSource) ensureClient(ctx context.Context) error {
	if s.client != nil {
		return nil
	}

	cfg, err := loadAWSConfig(ctx, &s.cfg)
	if err != nil {
		return err
	}

	s.client = ssm.NewFromConfig(cfg)
	return nil
}

// Load implements the source.Source interface.
// Fetches the parameter from SSM and caches the version for change detection.
func (s *SSMSource) Load(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := s.ensureClient(ctx); err != nil {
		return nil, err
	}

	result, err := s.client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(s.name),
		WithDecryption: aws.Bool(s.withDecrypt),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter %q: %w", s.name, err)
	}

	if result.Parameter == nil || result.Parameter.Value == nil {
		return nil, fmt.Errorf("parameter %q has no value", s.name)
	}

	// Cache version for change detection
	s.mu.Lock()
	s.lastVersion = result.Parameter.Version
	s.mu.Unlock()

	return []byte(*result.Parameter.Value), nil
}

// Save implements the source.Source interface.
// SSM source is read-only; this always returns ErrSaveNotSupported.
func (s *SSMSource) Save(ctx context.Context, updateFunc source.UpdateFunc) error {
	return source.ErrSaveNotSupported
}

// CanSave returns false because SSM sources do not support saving.
func (s *SSMSource) CanSave() bool {
	return false
}

// Name returns the SSM parameter name.
func (s *SSMSource) Name() string {
	return s.name
}

// Watch implements the source.WatchableSource interface.
// Returns a PollingWatcher that checks parameter version for changes.
func (s *SSMSource) Watch() (watcher.Watcher, error) {
	return watcher.NewPolling(watcher.PollHandlerFunc(s.poll)), nil
}

// poll checks for changes using parameter metadata and returns new data if changed.
func (s *SSMSource) poll(ctx context.Context) ([]byte, error) {
	if err := s.ensureClient(ctx); err != nil {
		return nil, err
	}

	result, err := s.client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(s.name),
		WithDecryption: aws.Bool(s.withDecrypt),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter %q: %w", s.name, err)
	}

	if result.Parameter == nil {
		return nil, fmt.Errorf("parameter %q not found", s.name)
	}

	s.mu.Lock()
	currentVersion := s.lastVersion
	s.mu.Unlock()

	// If version hasn't changed, return nil to indicate no change
	if result.Parameter.Version == currentVersion {
		return nil, nil
	}

	// Version changed - update cached version and return new value
	s.mu.Lock()
	s.lastVersion = result.Parameter.Version
	s.mu.Unlock()

	if result.Parameter.Value == nil {
		return nil, fmt.Errorf("parameter %q has no value", s.name)
	}

	return []byte(*result.Parameter.Value), nil
}
