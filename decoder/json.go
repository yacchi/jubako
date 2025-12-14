package decoder

import (
	"encoding/json"
	"fmt"
)

// JSON decodes a map[string]any into a target struct using JSON marshal/unmarshal.
//
// This is the default decoder used by jubako.Store.
func JSON(m map[string]any, target any) error {
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("failed to marshal map: %w", err)
	}

	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("failed to unmarshal to target type: %w", err)
	}

	return nil
}
