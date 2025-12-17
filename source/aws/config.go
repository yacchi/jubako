// Package aws provides AWS-based configuration sources (S3, SSM Parameter Store, AppConfig).
// These sources use efficient change detection via polling (ETag for S3, Version for Parameter Store,
// token-based for AppConfig).
package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

// Option is a marker interface for all AWS source options.
// Only types that implement this interface can be passed to NewXxxSource functions.
// This provides compile-time type safety for option parameters.
type Option interface {
	awsSourceOption()
}

// clientConfig holds shared AWS client configuration.
type clientConfig struct {
	awsConfig *aws.Config
}

// ClientOption configures AWS client behavior.
// It implements the Option interface and can be used with any AWS source.
type ClientOption func(*clientConfig)

// awsSourceOption implements the Option interface.
func (ClientOption) awsSourceOption() {}

// WithAWSConfig sets a custom AWS configuration.
// If not provided, the default configuration is loaded from the environment.
//
// Example:
//
//	cfg, _ := config.LoadDefaultConfig(ctx,
//	    config.WithRegion("us-west-2"),
//	)
//	src := aws.NewS3Source("bucket", "key", aws.WithAWSConfig(cfg))
func WithAWSConfig(cfg aws.Config) ClientOption {
	return func(c *clientConfig) {
		c.awsConfig = &cfg
	}
}

// loadAWSConfig returns the AWS config, loading the default if not set.
func loadAWSConfig(ctx context.Context, cfg *clientConfig) (aws.Config, error) {
	if cfg.awsConfig != nil {
		return *cfg.awsConfig, nil
	}

	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return aws.Config{}, fmt.Errorf("failed to load AWS config: %w", err)
	}
	return awsCfg, nil
}
