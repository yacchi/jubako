package aws

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/yacchi/jubako/source"
	"github.com/yacchi/jubako/watcher"
)

func TestParameterStoreSource_Type(t *testing.T) {
	src := NewParameterStoreSource("/test/param")
	if got := src.Type(); got != TypeParameterStore {
		t.Errorf("Type() = %v, want %v", got, TypeParameterStore)
	}
}

func TestParameterStoreSource_CanSave(t *testing.T) {
	src := NewParameterStoreSource("/test/param")
	if src.CanSave() {
		t.Error("CanSave() = true, want false")
	}
}

func TestParameterStoreSource_Save(t *testing.T) {
	src := NewParameterStoreSource("/test/param")
	err := src.Save(context.Background(), func([]byte) ([]byte, error) {
		return nil, nil
	})
	if err != source.ErrSaveNotSupported {
		t.Errorf("Save() error = %v, want %v", err, source.ErrSaveNotSupported)
	}
}

func TestParameterStoreSource_Name(t *testing.T) {
	src := NewParameterStoreSource("/app/config")
	if got := src.Name(); got != "/app/config" {
		t.Errorf("Name() = %v, want %v", got, "/app/config")
	}
}

func TestParameterStoreSource_Watch(t *testing.T) {
	src := NewParameterStoreSource("/test/param")

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

func TestParameterStoreSource_Load_CancelledContext(t *testing.T) {
	src := NewParameterStoreSource("/test/param")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := src.Load(ctx)
	if err == nil {
		t.Error("Load() with cancelled context should return error")
	}
}

func TestParameterStoreSource_WithOptions(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}

	src := NewParameterStoreSource("/test/param",
		WithAWSConfig(cfg),
		WithDecryption(true),
	)

	if src.cfg.awsConfig == nil {
		t.Error("WithAWSConfig did not set config")
	}
	if src.cfg.awsConfig.Region != "us-east-1" {
		t.Errorf("Region = %v, want %v", src.cfg.awsConfig.Region, "us-east-1")
	}
	if !src.withDecrypt {
		t.Error("WithDecryption(true) did not enable decryption")
	}
}

func TestParameterStoreSource_WithParameterStoreClient(t *testing.T) {
	client := &ssm.Client{}
	src := NewParameterStoreSource("/test/param", WithParameterStoreClient(client))

	if src.client != client {
		t.Error("WithParameterStoreClient did not set client")
	}
}

func TestParameterStoreSource_WithDecryption(t *testing.T) {
	tests := []struct {
		name    string
		decrypt bool
	}{
		{"enabled", true},
		{"disabled", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := NewParameterStoreSource("/test/param", WithDecryption(tt.decrypt))
			if src.withDecrypt != tt.decrypt {
				t.Errorf("withDecrypt = %v, want %v", src.withDecrypt, tt.decrypt)
			}
		})
	}
}

// Integration tests - only run when environment variables are set.
// Set JUBAKO_TEST_PARAMETER_STORE_NAME to run these tests.
func TestParameterStoreSource_Integration(t *testing.T) {
	paramName := os.Getenv("JUBAKO_TEST_PARAMETER_STORE_NAME")
	if paramName == "" {
		t.Skip("Skipping integration test: JUBAKO_TEST_PARAMETER_STORE_NAME not set")
	}

	ctx := context.Background()

	// Load default AWS config
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		t.Fatalf("LoadDefaultConfig() error = %v", err)
	}

	decryption := os.Getenv("JUBAKO_TEST_PARAMETER_STORE_DECRYPT") == "true"
	src := NewParameterStoreSource(paramName, WithAWSConfig(cfg), WithDecryption(decryption))

	t.Run("Load", func(t *testing.T) {
		data, err := src.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if len(data) == 0 {
			t.Error("Load() returned empty data")
		}
		t.Logf("Loaded %d bytes from Parameter Store %s", len(data), paramName)
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
