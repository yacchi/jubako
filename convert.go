package jubako

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// ValueConverter is a function that converts a value to the expected type.
// It is called only when the value's type doesn't match the target type.
//
// Parameters:
//   - path: JSONPointer path (e.g., "/server/port")
//   - value: the value to convert
//   - targetType: expected type from the struct definition
//
// Returns the converted value, or an error if conversion fails.
// If conversion is not possible, return the original value unchanged
// and let the JSON decoder handle the error.
type ValueConverter func(path string, value any, targetType reflect.Type) (any, error)

// DefaultValueConverter handles basic type conversions similar to mapstructure's
// WeaklyTypedInput mode. It converts between common types:
//
//   - string → bool: "true", "1", "yes", "on" → true; "false", "0", "no", "off" → false
//   - string → int/int8/int16/int32/int64: parsed via strconv.ParseInt
//   - string → uint/uint8/uint16/uint32/uint64: parsed via strconv.ParseUint
//   - string → float32/float64: parsed via strconv.ParseFloat
//   - bool → string: true → "true", false → "false"
//   - bool → int: true → 1, false → 0
//   - int/float → string: formatted via fmt.Sprint
//   - int/float → bool: 0 → false, non-zero → true
//   - []any → []T: element-wise conversion (if elements are convertible)
//
// If conversion is not supported, the original value is returned unchanged.
var DefaultValueConverter ValueConverter = defaultConvert

// defaultConvert implements the default value conversion logic.
func defaultConvert(path string, value any, targetType reflect.Type) (any, error) {
	if value == nil {
		return nil, nil
	}

	valueType := reflect.TypeOf(value)

	// Already the correct type
	if valueType == targetType {
		return value, nil
	}

	// Handle pointer target types
	if targetType.Kind() == reflect.Ptr {
		// Convert to the element type, then wrap in pointer
		elemType := targetType.Elem()
		converted, err := defaultConvert(path, value, elemType)
		if err != nil {
			return nil, err
		}
		// Create a pointer to the converted value
		ptr := reflect.New(elemType)
		ptr.Elem().Set(reflect.ValueOf(converted))
		return ptr.Interface(), nil
	}

	// Dispatch based on target type kind
	switch targetType.Kind() {
	case reflect.Bool:
		return convertToBool(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return convertToInt(value, targetType)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return convertToUint(value, targetType)
	case reflect.Float32, reflect.Float64:
		return convertToFloat(value, targetType)
	case reflect.String:
		return convertToString(value)
	case reflect.Slice:
		return convertToSlice(path, value, targetType)
	case reflect.Map:
		return convertToMap(path, value, targetType)
	}

	// No conversion available, return as-is
	return value, nil
}

// convertToBool converts various types to bool.
func convertToBool(value any) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		return stringToBool(v)
	case int, int8, int16, int32, int64:
		return reflect.ValueOf(v).Int() != 0, nil
	case uint, uint8, uint16, uint32, uint64:
		return reflect.ValueOf(v).Uint() != 0, nil
	case float32, float64:
		return reflect.ValueOf(v).Float() != 0, nil
	default:
		return false, fmt.Errorf("cannot convert %T to bool", value)
	}
}

// stringToBool converts a string to bool with common truthy/falsy values.
func stringToBool(s string) (bool, error) {
	lower := strings.ToLower(strings.TrimSpace(s))
	switch lower {
	case "true", "1", "yes", "on", "t", "y":
		return true, nil
	case "false", "0", "no", "off", "f", "n", "":
		return false, nil
	default:
		return false, fmt.Errorf("cannot convert %q to bool", s)
	}
}

// convertToInt converts various types to int types.
func convertToInt(value any, targetType reflect.Type) (any, error) {
	var i int64
	var err error

	switch v := value.(type) {
	case string:
		i, err = strconv.ParseInt(strings.TrimSpace(v), 0, 64)
		if err != nil {
			// Try parsing as float first (handles "3.14" -> 3)
			var f float64
			f, err = strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil {
				return nil, fmt.Errorf("cannot convert %q to int: %w", v, err)
			}
			i = int64(f)
		}
	case bool:
		if v {
			i = 1
		} else {
			i = 0
		}
	case int:
		i = int64(v)
	case int8:
		i = int64(v)
	case int16:
		i = int64(v)
	case int32:
		i = int64(v)
	case int64:
		i = v
	case uint:
		i = int64(v)
	case uint8:
		i = int64(v)
	case uint16:
		i = int64(v)
	case uint32:
		i = int64(v)
	case uint64:
		i = int64(v)
	case float32:
		i = int64(v)
	case float64:
		i = int64(v)
	default:
		return nil, fmt.Errorf("cannot convert %T to int", value)
	}

	// Convert to the specific target type
	result := reflect.New(targetType).Elem()
	result.SetInt(i)
	return result.Interface(), nil
}

// convertToUint converts various types to uint types.
func convertToUint(value any, targetType reflect.Type) (any, error) {
	var u uint64
	var err error

	switch v := value.(type) {
	case string:
		u, err = strconv.ParseUint(strings.TrimSpace(v), 0, 64)
		if err != nil {
			// Try parsing as float first
			var f float64
			f, err = strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil {
				return nil, fmt.Errorf("cannot convert %q to uint: %w", v, err)
			}
			if f < 0 {
				return nil, fmt.Errorf("cannot convert negative value %q to uint", v)
			}
			u = uint64(f)
		}
	case bool:
		if v {
			u = 1
		} else {
			u = 0
		}
	case int:
		u = uint64(v)
	case int8:
		u = uint64(v)
	case int16:
		u = uint64(v)
	case int32:
		u = uint64(v)
	case int64:
		u = uint64(v)
	case uint:
		u = uint64(v)
	case uint8:
		u = uint64(v)
	case uint16:
		u = uint64(v)
	case uint32:
		u = uint64(v)
	case uint64:
		u = v
	case float32:
		u = uint64(v)
	case float64:
		u = uint64(v)
	default:
		return nil, fmt.Errorf("cannot convert %T to uint", value)
	}

	// Convert to the specific target type
	result := reflect.New(targetType).Elem()
	result.SetUint(u)
	return result.Interface(), nil
}

// convertToFloat converts various types to float types.
func convertToFloat(value any, targetType reflect.Type) (any, error) {
	var f float64
	var err error

	switch v := value.(type) {
	case string:
		f, err = strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return nil, fmt.Errorf("cannot convert %q to float: %w", v, err)
		}
	case bool:
		if v {
			f = 1
		} else {
			f = 0
		}
	case int:
		f = float64(v)
	case int8:
		f = float64(v)
	case int16:
		f = float64(v)
	case int32:
		f = float64(v)
	case int64:
		f = float64(v)
	case uint:
		f = float64(v)
	case uint8:
		f = float64(v)
	case uint16:
		f = float64(v)
	case uint32:
		f = float64(v)
	case uint64:
		f = float64(v)
	case float32:
		f = float64(v)
	case float64:
		f = v
	default:
		return nil, fmt.Errorf("cannot convert %T to float", value)
	}

	// Convert to the specific target type
	result := reflect.New(targetType).Elem()
	result.SetFloat(f)
	return result.Interface(), nil
}

// convertToString converts various types to string.
func convertToString(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case bool:
		return strconv.FormatBool(v), nil
	case int:
		return strconv.FormatInt(int64(v), 10), nil
	case int8:
		return strconv.FormatInt(int64(v), 10), nil
	case int16:
		return strconv.FormatInt(int64(v), 10), nil
	case int32:
		return strconv.FormatInt(int64(v), 10), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case uint:
		return strconv.FormatUint(uint64(v), 10), nil
	case uint8:
		return strconv.FormatUint(uint64(v), 10), nil
	case uint16:
		return strconv.FormatUint(uint64(v), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(v), 10), nil
	case uint64:
		return strconv.FormatUint(v, 10), nil
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	default:
		return fmt.Sprint(v), nil
	}
}

// convertToSlice converts []any to []T with element-wise conversion.
func convertToSlice(path string, value any, targetType reflect.Type) (any, error) {
	srcSlice, ok := value.([]any)
	if !ok {
		// Try to handle other slice types via reflection
		srcVal := reflect.ValueOf(value)
		if srcVal.Kind() != reflect.Slice {
			return value, nil // Not a slice, return as-is
		}
		srcSlice = make([]any, srcVal.Len())
		for i := 0; i < srcVal.Len(); i++ {
			srcSlice[i] = srcVal.Index(i).Interface()
		}
	}

	elemType := targetType.Elem()
	result := reflect.MakeSlice(targetType, len(srcSlice), len(srcSlice))

	for i, elem := range srcSlice {
		elemPath := fmt.Sprintf("%s/%d", path, i)
		converted, err := defaultConvert(elemPath, elem, elemType)
		if err != nil {
			return nil, fmt.Errorf("cannot convert slice element at index %d: %w", i, err)
		}
		if converted != nil {
			result.Index(i).Set(reflect.ValueOf(converted))
		}
	}

	return result.Interface(), nil
}

// convertToMap converts map[string]any to map[K]V with element-wise conversion.
func convertToMap(path string, value any, targetType reflect.Type) (any, error) {
	srcMap, ok := value.(map[string]any)
	if !ok {
		return value, nil // Not a map[string]any, return as-is
	}

	keyType := targetType.Key()
	valueType := targetType.Elem()

	// Only handle string keys for now
	if keyType.Kind() != reflect.String {
		return value, nil
	}

	result := reflect.MakeMap(targetType)

	for k, v := range srcMap {
		elemPath := fmt.Sprintf("%s/%s", path, k)
		converted, err := defaultConvert(elemPath, v, valueType)
		if err != nil {
			return nil, fmt.Errorf("cannot convert map value for key %q: %w", k, err)
		}
		if converted != nil {
			result.SetMapIndex(reflect.ValueOf(k), reflect.ValueOf(converted))
		}
	}

	return result.Interface(), nil
}