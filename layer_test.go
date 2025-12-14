package jubako

import (
	"context"
	"testing"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/layer"
	"github.com/yacchi/jubako/layer/mapdata"
)

func TestLayerPriority_Constants(t *testing.T) {
	tests := []struct {
		name     string
		priority layer.Priority
		want     int
	}{
		{name: "PriorityDefaults", priority: PriorityDefaults, want: 0},
		{name: "PriorityUser", priority: PriorityUser, want: 10},
		{name: "PriorityProject", priority: PriorityProject, want: 20},
		{name: "PriorityEnv", priority: PriorityEnv, want: 30},
		{name: "PriorityFlags", priority: PriorityFlags, want: 40},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if int(tt.priority) != tt.want {
				t.Errorf("%s = %d, want %d", tt.name, tt.priority, tt.want)
			}
		})
	}
}

func TestLayerPriority_Ordering(t *testing.T) {
	// Verify that priorities are in ascending order
	priorities := []layer.Priority{
		PriorityDefaults,
		PriorityUser,
		PriorityProject,
		PriorityEnv,
		PriorityFlags,
	}

	for i := 0; i < len(priorities)-1; i++ {
		if priorities[i] >= priorities[i+1] {
			t.Errorf("Priority[%d] (%d) >= Priority[%d] (%d), expected ascending order",
				i, priorities[i], i+1, priorities[i+1])
		}
	}
}

func TestMapdataLayer_Creation(t *testing.T) {
	ctx := context.Background()
	data := map[string]any{"test": "value"}

	l := mapdata.New("test", data)

	if l.Name() != "test" {
		t.Errorf("Name() = %q, want %q", l.Name(), "test")
	}

	// Load and verify
	result, err := l.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if result == nil {
		t.Error("Load() returned nil")
	}
	if result["test"] != "value" {
		t.Errorf("Load()[test] = %v, want value", result["test"])
	}
}

func TestLayerPriority_CustomValues(t *testing.T) {
	// Test that custom priorities can be used
	// Use 15 which is between PriorityUser (10) and PriorityProject (20)
	customPriority := layer.Priority(15)

	if customPriority <= PriorityUser {
		t.Errorf("Custom priority %d should be greater than PriorityUser (%d)",
			customPriority, PriorityUser)
	}
	if customPriority >= PriorityProject {
		t.Errorf("Custom priority %d should be less than PriorityProject (%d)",
			customPriority, PriorityProject)
	}
}

func TestMapdataLayer_Load(t *testing.T) {
	ctx := context.Background()

	t.Run("load returns data", func(t *testing.T) {
		l := mapdata.New("test", map[string]any{"test": "value"})

		result, err := l.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if result == nil {
			t.Error("Load() returned nil")
		}
		if result["test"] != "value" {
			t.Errorf("Load()[test] = %v, want value", result["test"])
		}
	})

	t.Run("load with nested data", func(t *testing.T) {
		l := mapdata.New("test", map[string]any{
			"server": map[string]any{
				"host": "localhost",
				"port": 8080,
			},
		})

		result, err := l.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if result == nil {
			t.Error("Load() returned nil")
		}

		server, ok := result["server"].(map[string]any)
		if !ok {
			t.Fatal("result[server] is not a map")
		}
		if server["host"] != "localhost" {
			t.Errorf("server[host] = %v, want localhost", server["host"])
		}
	})
}

func TestMapdataLayer_Save(t *testing.T) {
	ctx := context.Background()

	t.Run("CanSave returns true for mapdata layer", func(t *testing.T) {
		l := mapdata.New("test", map[string]any{"test": "value"})

		if !l.CanSave() {
			t.Error("CanSave() should return true for mapdata layer")
		}
	})

	t.Run("save succeeds for mapdata layer", func(t *testing.T) {
		l := mapdata.New("test", map[string]any{"test": "value"})

		// Modify via Save with changeset
		changeset := document.JSONPatchSet{
			document.NewReplacePatch("/test", "modified"),
		}
		err := l.Save(ctx, changeset)
		if err != nil {
			t.Errorf("Save() error = %v, want nil", err)
		}

		// Load and verify the data was saved
		result, err := l.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if result["test"] != "modified" {
			t.Errorf("result[test] = %v, want modified", result["test"])
		}
	})
}

func TestMapdataLayer_ContextCancellation(t *testing.T) {
	ctx := context.Background()

	t.Run("context cancellation", func(t *testing.T) {
		l := mapdata.New("test", map[string]any{"test": "data"})
		canceledCtx, cancel := context.WithCancel(ctx)
		cancel() // Cancel immediately

		_, err := l.Load(canceledCtx)
		if err == nil {
			t.Error("Load() should return error with canceled context")
		}
	})
}
