package aws

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/yacchi/jubako/source"
	"github.com/yacchi/jubako/watcher"
)

func TestS3Source_Type(t *testing.T) {
	src := NewS3Source("test-bucket", "test-key")
	if got := src.Type(); got != TypeS3 {
		t.Errorf("Type() = %v, want %v", got, TypeS3)
	}
}

func TestS3Source_CanSave(t *testing.T) {
	src := NewS3Source("test-bucket", "test-key")
	if src.CanSave() {
		t.Error("CanSave() = true, want false")
	}
}

func TestS3Source_Save(t *testing.T) {
	src := NewS3Source("test-bucket", "test-key")
	err := src.Save(context.Background(), func([]byte) ([]byte, error) {
		return nil, nil
	})
	if err != source.ErrSaveNotSupported {
		t.Errorf("Save() error = %v, want %v", err, source.ErrSaveNotSupported)
	}
}

func TestS3Source_Bucket(t *testing.T) {
	src := NewS3Source("my-bucket", "my-key")
	if got := src.Bucket(); got != "my-bucket" {
		t.Errorf("Bucket() = %v, want %v", got, "my-bucket")
	}
}

func TestS3Source_Key(t *testing.T) {
	src := NewS3Source("my-bucket", "my-key")
	if got := src.Key(); got != "my-key" {
		t.Errorf("Key() = %v, want %v", got, "my-key")
	}
}

func TestS3Source_Watch(t *testing.T) {
	src := NewS3Source("test-bucket", "test-key")

	init, err := src.Watch()
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	if init == nil {
		t.Fatal("Watch() returned nil initializer")
	}

	// Create watcher with test params
	var mu sync.Mutex
	w, err := init(watcher.WatcherInitializerParams{
		Fetch: func(ctx context.Context) (bool, []byte, error) {
			return true, nil, nil
		},
		OpMu: &mu,
	})
	if err != nil {
		t.Fatalf("WatcherInitializer() error = %v", err)
	}
	if w.Type() != watcher.TypePolling {
		t.Errorf("Watch().Type() = %v, want %v", w.Type(), watcher.TypePolling)
	}
}

func TestS3Source_Load_CancelledContext(t *testing.T) {
	src := NewS3Source("test-bucket", "test-key")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := src.Load(ctx)
	if err == nil {
		t.Error("Load() with cancelled context should return error")
	}
}

func TestS3Source_WithOptions(t *testing.T) {
	cfg := aws.Config{Region: "us-west-2"}

	src := NewS3Source("bucket", "key",
		WithAWSConfig(cfg),
	)

	if src.cfg.awsConfig == nil {
		t.Error("WithAWSConfig did not set config")
	}
	if src.cfg.awsConfig.Region != "us-west-2" {
		t.Errorf("Region = %v, want %v", src.cfg.awsConfig.Region, "us-west-2")
	}
}

func TestS3Source_WithS3Client(t *testing.T) {
	client := &s3.Client{}
	src := NewS3Source("bucket", "key", WithS3Client(client))

	if src.client != client {
		t.Error("WithS3Client did not set client")
	}
}

// Integration tests - only run when environment variables are set.
// Set JUBAKO_TEST_S3_BUCKET and JUBAKO_TEST_S3_KEY to run these tests.
func TestS3Source_Integration(t *testing.T) {
	bucket := os.Getenv("JUBAKO_TEST_S3_BUCKET")
	key := os.Getenv("JUBAKO_TEST_S3_KEY")
	if bucket == "" || key == "" {
		t.Skip("Skipping integration test: JUBAKO_TEST_S3_BUCKET and JUBAKO_TEST_S3_KEY not set")
	}

	ctx := context.Background()

	// Load default AWS config
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		t.Fatalf("LoadDefaultConfig() error = %v", err)
	}

	src := NewS3Source(bucket, key, WithAWSConfig(cfg))

	t.Run("Load", func(t *testing.T) {
		data, err := src.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if len(data) == 0 {
			t.Error("Load() returned empty data")
		}
		t.Logf("Loaded %d bytes from s3://%s/%s", len(data), bucket, key)
	})

	t.Run("Watch", func(t *testing.T) {
		init, err := src.Watch()
		if err != nil {
			t.Fatalf("Watch() error = %v", err)
		}

		// Create watcher with test params
		var mu sync.Mutex
		cfg := watcher.NewWatchConfig(watcher.WithPollInterval(1 * time.Second))
		w, err := init(watcher.WatcherInitializerParams{
			Fetch: func(ctx context.Context) (bool, []byte, error) {
				return true, nil, nil
			},
			OpMu:   &mu,
			Config: cfg,
		})
		if err != nil {
			t.Fatalf("WatcherInitializer() error = %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := w.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer w.Stop(context.Background())

		// Just verify the watcher starts and stops without error
		t.Log("Watcher started successfully")
	})
}
