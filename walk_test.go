package jubako

import (
	"context"
	"sort"
	"testing"

	"github.com/yacchi/jubako/layer/mapdata"
)

func TestStore_Walk_IterationOrder(t *testing.T) {
	ctx := context.Background()

	t.Run("walk iteration order should be sorted by path", func(t *testing.T) {
		store := New[map[string]any]()

		// Add a layer with multiple keys
		data := map[string]any{
			"z": 1,
			"a": 2,
			"c": 3,
			"b": 4,
			"y": 5,
			"d": 6,
		}
		err := store.Add(mapdata.New("defaults", data), WithPriority(PriorityDefaults))
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}

		err = store.Load(ctx)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// Run Walk multiple times to check stability and sort order
		for i := 0; i < 5; i++ {
			var paths []string
			store.Walk(func(ctx WalkContext) bool {
				paths = append(paths, ctx.Path)
				return true
			})

			if !sort.StringsAreSorted(paths) {
				t.Errorf("Walk iteration #%d is not sorted: %v", i, paths)
			}
		}
	})
}
