package jubako

import (
	"fmt"
	"log"
	"reflect"
	"strings"

	"github.com/yacchi/jubako/internal/tag"
)

// SensitiveWarning represents a warning about incorrect sensitive tag usage.
type SensitiveWarning struct {
	// TypePath is the full path to the type (e.g., "AppConfig.Credentials")
	TypePath string
	// FieldName is the field that has the incorrect sensitive tag
	FieldName string
	// FieldType is the type kind (struct, map, slice, etc.)
	FieldType string
	// Message is a human-readable description of the warning
	Message string
}

func (w SensitiveWarning) String() string {
	return fmt.Sprintf("jubako: WARNING: %s.%s (%s): %s", w.TypePath, w.FieldName, w.FieldType, w.Message)
}

// SensitiveWarningHandler is called when a sensitive tag is found on a non-leaf type.
type SensitiveWarningHandler func(warning SensitiveWarning)

// defaultSensitiveWarningHandler logs warnings to stderr using the standard log package.
var defaultSensitiveWarningHandler SensitiveWarningHandler = func(w SensitiveWarning) {
	log.Println(w.String())
}

// sensitiveWarningHandler is the current handler for sensitive warnings.
// Can be set via SetSensitiveWarningHandler.
var sensitiveWarningHandler = defaultSensitiveWarningHandler

// SetSensitiveWarningHandler sets a custom handler for sensitive tag warnings.
// Pass nil to disable warnings.
// This function is not thread-safe and should be called during initialization.
//
// Example:
//
//	// Disable warnings
//	jubako.SetSensitiveWarningHandler(nil)
//
//	// Custom handler
//	jubako.SetSensitiveWarningHandler(func(w jubako.SensitiveWarning) {
//	    myLogger.Warn(w.String())
//	})
func SetSensitiveWarningHandler(handler SensitiveWarningHandler) {
	sensitiveWarningHandler = handler
}

// checkSensitiveOnNonLeaf checks if the sensitive tag is applied to a non-leaf type
// and emits a warning if so.
func checkSensitiveOnNonLeaf(typePath string, field reflect.StructField, tagInfo tag.FieldInfo) {
	if tagInfo.Sensitive != tag.SensitiveExplicit {
		return
	}

	if sensitiveWarningHandler == nil {
		return
	}

	fieldType := tagInfo.FieldType
	kind := fieldType.Kind()

	// Check if this is a non-leaf type (container type)
	var typeKind string
	switch kind {
	case reflect.Struct:
		typeKind = "struct"
	case reflect.Map:
		typeKind = "map"
	case reflect.Slice:
		typeKind = "slice"
	case reflect.Array:
		typeKind = "array"
	default:
		// Leaf type - no warning needed
		return
	}

	// Build field type description for the warning
	var typeDesc strings.Builder
	typeDesc.WriteString(typeKind)
	if kind == reflect.Map {
		typeDesc.WriteString("[")
		typeDesc.WriteString(fieldType.Key().String())
		typeDesc.WriteString("]")
		typeDesc.WriteString(fieldType.Elem().String())
	} else if kind == reflect.Slice || kind == reflect.Array {
		typeDesc.WriteString(" of ")
		typeDesc.WriteString(fieldType.Elem().String())
	} else if kind == reflect.Struct {
		typeDesc.WriteString(" ")
		typeDesc.WriteString(fieldType.String())
	}

	sensitiveWarningHandler(SensitiveWarning{
		TypePath:  typePath,
		FieldName: field.Name,
		FieldType: typeDesc.String(),
		Message:   "sensitive tag should only be applied to leaf fields (string, int, etc.), not container types. Mark individual leaf fields as sensitive instead.",
	})
}
