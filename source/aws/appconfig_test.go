package aws

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/appconfigdata"
	"github.com/yacchi/jubako/source"
	"github.com/yacchi/jubako/watcher"
)

func TestAppConfigSource_Type(t *testing.T) {
	src := NewAppConfigSource("app", "env", "profile")
	if got := src.Type(); got != TypeAppConfig {
		t.Errorf("Type() = %v, want %v", got, TypeAppConfig)
	}
}

func TestAppConfigSource_CanSave(t *testing.T) {
	src := NewAppConfigSource("app", "env", "profile")
	if src.CanSave() {
		t.Error("CanSave() = true, want false")
	}
}

func TestAppConfigSource_Save(t *testing.T) {
	src := NewAppConfigSource("app", "env", "profile")
	err := src.Save(context.Background(), func([]byte) ([]byte, error) {
		return nil, nil
	})
	if err != source.ErrSaveNotSupported {
		t.Errorf("Save() error = %v, want %v", err, source.ErrSaveNotSupported)
	}
}

func TestAppConfigSource_Accessors(t *testing.T) {
	src := NewAppConfigSource("MyApp", "Production", "MainConfig")

	if got := src.Application(); got != "MyApp" {
		t.Errorf("Application() = %v, want %v", got, "MyApp")
	}
	if got := src.Environment(); got != "Production" {
		t.Errorf("Environment() = %v, want %v", got, "Production")
	}
	if got := src.ConfigurationProfile(); got != "MainConfig" {
		t.Errorf("ConfigurationProfile() = %v, want %v", got, "MainConfig")
	}
}

func TestAppConfigSource_Watch(t *testing.T) {
	src := NewAppConfigSource("app", "env", "profile")

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

func TestAppConfigSource_Load_CancelledContext(t *testing.T) {
	src := NewAppConfigSource("app", "env", "profile")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := src.Load(ctx)
	if err == nil {
		t.Error("Load() with cancelled context should return error")
	}
}

func TestAppConfigSource_WithOptions(t *testing.T) {
	cfg := aws.Config{Region: "ap-northeast-1"}

	src := NewAppConfigSource("app", "env", "profile",
		WithAWSConfig(cfg),
	)

	if src.cfg.awsConfig == nil {
		t.Error("WithAWSConfig did not set config")
	}
	if src.cfg.awsConfig.Region != "ap-northeast-1" {
		t.Errorf("Region = %v, want %v", src.cfg.awsConfig.Region, "ap-northeast-1")
	}
}

func TestAppConfigSource_WithAppConfigClient(t *testing.T) {
	client := &appconfigdata.Client{}
	src := NewAppConfigSource("app", "env", "profile", WithAppConfigClient(client))

	if src.client != client {
		t.Error("WithAppConfigClient did not set client")
	}
}

// Integration tests - only run when environment variables are set.
// Set JUBAKO_TEST_APPCONFIG_APPLICATION, JUBAKO_TEST_APPCONFIG_ENVIRONMENT,
// and JUBAKO_TEST_APPCONFIG_PROFILE to run these tests.
func TestAppConfigSource_Integration(t *testing.T) {
	app := os.Getenv("JUBAKO_TEST_APPCONFIG_APPLICATION")
	env := os.Getenv("JUBAKO_TEST_APPCONFIG_ENVIRONMENT")
	profile := os.Getenv("JUBAKO_TEST_APPCONFIG_PROFILE")

	if app == "" || env == "" || profile == "" {
		t.Skip("Skipping integration test: JUBAKO_TEST_APPCONFIG_APPLICATION, JUBAKO_TEST_APPCONFIG_ENVIRONMENT, and JUBAKO_TEST_APPCONFIG_PROFILE not set")
	}

	ctx := context.Background()

	// Load default AWS config
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		t.Fatalf("LoadDefaultConfig() error = %v", err)
	}

	src := NewAppConfigSource(app, env, profile, WithAWSConfig(cfg))

	t.Run("Load", func(t *testing.T) {
		data, err := src.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if len(data) == 0 {
			t.Error("Load() returned empty data")
		}
		t.Logf("Loaded %d bytes from AppConfig %s/%s/%s", len(data), app, env, profile)
	})

	t.Run("LoadMultipleTimes", func(t *testing.T) {
		// Test that subsequent loads work correctly (token management)
		for i := 0; i < 3; i++ {
			data, err := src.Load(ctx)
			if err != nil {
				t.Fatalf("Load() iteration %d error = %v", i, err)
			}
			if len(data) == 0 {
				t.Errorf("Load() iteration %d returned empty data", i)
			}
		}
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
