package jubako

import "errors"

// Sensitivity validation errors.
var (
	// ErrSensitiveFieldToNormalLayer is returned when attempting to write
	// a sensitive field to a non-sensitive layer.
	ErrSensitiveFieldToNormalLayer = errors.New("cannot write sensitive field to non-sensitive layer")
)

// SensitiveMaskFunc is a function that masks sensitive values.
// It receives the original value and returns the masked value.
// The function is called for sensitive fields when accessed through GetAt, Walk, etc.
//
// Example:
//
//	func maskHandler(value any) any {
//	    return "********"
//	}
type SensitiveMaskFunc func(value any) any

// DefaultMaskString is the default mask string used by WithSensitiveMaskString
// when no custom string is provided.
const DefaultMaskString = "********"

// validateSensitivity checks if writing to the given path in a layer is allowed.
// Returns an error if:
// - The path is sensitive but the layer is not sensitive (ErrSensitiveFieldToNormalLayer)
//
// Note: Non-sensitive fields CAN be written to sensitive layers. This allows storing
// related but non-sensitive data (e.g., account IDs) alongside sensitive data in
// secure storage locations.
func validateSensitivity(table *MappingTable, path string, layerSensitive bool) error {
	fieldSensitive := table.IsSensitive(path)

	if fieldSensitive && !layerSensitive {
		return ErrSensitiveFieldToNormalLayer
	}
	return nil
}
