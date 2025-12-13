package jubako

import (
	"github.com/yacchi/jubako/layer"
)

// LayerPriority is an alias for layer.Priority.
type LayerPriority = layer.Priority

// LayerName is an alias for layer.Name.
type LayerName = layer.Name

// Priority constants for common configuration layer priorities.
// Higher values take precedence during merging.
// These use a step of 10, matching defaultPriorityStep for consistency
// with auto-assigned priorities.
const (
	// PriorityDefaults is the lowest priority, used for default values.
	PriorityDefaults LayerPriority = 0

	// PriorityUser is for user-level configuration (e.g., ~/.config).
	PriorityUser LayerPriority = 10

	// PriorityProject is for project-level configuration (e.g., .app.yaml).
	PriorityProject LayerPriority = 20

	// PriorityEnv is for environment variables.
	PriorityEnv LayerPriority = 30

	// PriorityFlags is the highest priority, used for command-line flags.
	PriorityFlags LayerPriority = 40
)
