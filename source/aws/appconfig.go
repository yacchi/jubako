package aws

import (
	"context"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/appconfigdata"
	"github.com/yacchi/jubako/source"
	"github.com/yacchi/jubako/watcher"
)

// AppConfigSource loads configuration from AWS AppConfig.
// This source is read-only; Save operations return ErrSaveNotSupported.
// Change detection is built into the AppConfig API using configuration tokens.
//
// Note: AppConfigSource does not implement its own synchronization. When used with
// layer.New(), the Layer provides operation-level synchronization between
// Load, Save, and poll operations.
type AppConfigSource struct {
	application          string
	environment          string
	configurationProfile string
	cfg                  clientConfig
	client               *appconfigdata.Client

	clientInit    sync.Once
	clientInitErr error
}

// Ensure AppConfigSource implements the source.Source interface.
var _ source.Source = (*AppConfigSource)(nil)

// Ensure AppConfigSource implements the source.WatchableSource interface.
var _ source.WatchableSource = (*AppConfigSource)(nil)

// TypeAppConfig is the source type identifier for AppConfig sources.
const TypeAppConfig source.SourceType = "appconfig"

// AppConfigOption configures an AppConfigSource.
// It implements the Option interface.
type AppConfigOption func(*AppConfigSource)

// awsSourceOption implements the Option interface.
func (AppConfigOption) awsSourceOption() {}

// WithAppConfigClient sets a custom AppConfig data client.
// This overrides WithAWSConfig for the AppConfig client.
func WithAppConfigClient(client *appconfigdata.Client) AppConfigOption {
	return func(s *AppConfigSource) {
		s.client = client
	}
}

// NewAppConfigSource creates an AppConfig source for the given application, environment,
// and configuration profile.
//
// Example:
//
//	src := aws.NewAppConfigSource("MyApp", "Production", "MainConfig")
//	src := aws.NewAppConfigSource("MyApp", "Production", "MainConfig", aws.WithAWSConfig(cfg))
//	src := aws.NewAppConfigSource("MyApp", "Production", "MainConfig", aws.WithAppConfigClient(customClient))
func NewAppConfigSource(application, environment, configurationProfile string, opts ...Option) *AppConfigSource {
	s := &AppConfigSource{
		application:          application,
		environment:          environment,
		configurationProfile: configurationProfile,
	}

	for _, opt := range opts {
		switch o := opt.(type) {
		case ClientOption:
			o(&s.cfg)
		case AppConfigOption:
			o(s)
		}
	}

	return s
}

// ensureClient creates a default AppConfig data client if one was not provided.
func (s *AppConfigSource) ensureClient(ctx context.Context) error {
	if s.client != nil {
		return nil
	}

	s.clientInit.Do(func() {
		cfg, err := loadAWSConfig(ctx, &s.cfg)
		if err != nil {
			s.clientInitErr = err
			return
		}
		s.client = appconfigdata.NewFromConfig(cfg)
	})
	return s.clientInitErr
}

// startSession starts a new configuration session and returns the initial token.
func (s *AppConfigSource) startSession(ctx context.Context) (string, error) {
	result, err := s.client.StartConfigurationSession(ctx, &appconfigdata.StartConfigurationSessionInput{
		ApplicationIdentifier:          aws.String(s.application),
		EnvironmentIdentifier:          aws.String(s.environment),
		ConfigurationProfileIdentifier: aws.String(s.configurationProfile),
	})
	if err != nil {
		return "", fmt.Errorf("failed to start configuration session for %s/%s/%s: %w",
			s.application, s.environment, s.configurationProfile, err)
	}

	if result.InitialConfigurationToken == nil {
		return "", fmt.Errorf("no initial configuration token returned")
	}

	return *result.InitialConfigurationToken, nil
}

// Load implements the source.Source interface.
// Fetches the configuration from AppConfig.
func (s *AppConfigSource) Load(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := s.ensureClient(ctx); err != nil {
		return nil, err
	}

	// Start a new session for each load to avoid shared mutable state.
	token, err := s.startSession(ctx)
	if err != nil {
		return nil, err
	}

	result, err := s.client.GetLatestConfiguration(ctx, &appconfigdata.GetLatestConfigurationInput{
		ConfigurationToken: aws.String(token),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get configuration for %s/%s/%s: %w",
			s.application, s.environment, s.configurationProfile, err)
	}

	if len(result.Configuration) == 0 {
		return nil, fmt.Errorf("no configuration data available for %s/%s/%s",
			s.application, s.environment, s.configurationProfile)
	}

	return result.Configuration, nil
}

// Save implements the source.Source interface.
// AppConfig source is read-only; this always returns ErrSaveNotSupported.
func (s *AppConfigSource) Save(ctx context.Context, updateFunc source.UpdateFunc) error {
	return source.ErrSaveNotSupported
}

// CanSave returns false because AppConfig sources do not support saving.
func (s *AppConfigSource) CanSave() bool {
	return false
}

// Type returns the source type identifier.
func (s *AppConfigSource) Type() source.SourceType {
	return TypeAppConfig
}

// Application returns the AppConfig application identifier.
func (s *AppConfigSource) Application() string {
	return s.application
}

// Environment returns the AppConfig environment identifier.
func (s *AppConfigSource) Environment() string {
	return s.environment
}

// ConfigurationProfile returns the AppConfig configuration profile identifier.
func (s *AppConfigSource) ConfigurationProfile() string {
	return s.configurationProfile
}

// Watch implements the source.WatchableSource interface.
// Returns a WatcherInitializer that creates a PollingWatcher using AppConfig's
// built-in token-based change detection.
func (s *AppConfigSource) Watch() (watcher.WatcherInitializer, error) {
	var token string

	pollOnce := func(ctx context.Context) (bool, []byte, error) {
		if err := s.ensureClient(ctx); err != nil {
			return false, nil, err
		}

		// Start a session if we don't have a token yet.
		if token == "" {
			t, err := s.startSession(ctx)
			if err != nil {
				return false, nil, err
			}
			token = t
		}

		result, err := s.client.GetLatestConfiguration(ctx, &appconfigdata.GetLatestConfigurationInput{
			ConfigurationToken: aws.String(token),
		})
		if err != nil {
			return false, nil, fmt.Errorf("failed to get configuration for %s/%s/%s: %w",
				s.application, s.environment, s.configurationProfile, err)
		}

		// Advance token for next poll.
		if result.NextPollConfigurationToken != nil {
			token = *result.NextPollConfigurationToken
		}

		// AppConfig returns empty configuration when nothing has changed.
		if len(result.Configuration) == 0 {
			return false, nil, nil
		}
		return true, result.Configuration, nil
	}

	return watcher.NewPolling(pollOnce), nil
}
