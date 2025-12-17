package paths

import (
	"fmt"
	"go/types"
	"reflect"
	"regexp"
	"strings"
	"unicode"
)

const jubakoTagName = "jubako"

// PathInfo represents a complete path to a configuration value.
type PathInfo struct {
	// JSONPointer is the full path, e.g., "/hosts/{key}/url"
	JSONPointer string
	// ConstName is the constant name if static, e.g., "PathServerPort"
	ConstName string
	// FuncName is the function name if dynamic, e.g., "HostsURL"
	FuncName string
	// DynamicParams holds parameters for dynamic paths
	DynamicParams []ParamInfo
	// FieldName is the original Go field name
	FieldName string
	// Comment is a description for documentation
	Comment string
}

// ParamInfo describes a dynamic parameter.
type ParamInfo struct {
	Name string // key, index, key2, index2...
	Type string // "string" for map, "int" for slice
}

// AnalysisResult holds all discovered paths.
type AnalysisResult struct {
	Paths []PathInfo
}

// analysisContext tracks state during recursive analysis.
type analysisContext struct {
	currentPath    string
	dynamicParams  []ParamInfo
	tagName        string
	mapKeyCount    int
	sliceIdxCount  int
}

func newAnalysisContext(tagName string) *analysisContext {
	return &analysisContext{
		currentPath:   "",
		dynamicParams: nil,
		tagName:       tagName,
	}
}

func (ctx *analysisContext) withPath(segment string) *analysisContext {
	newCtx := &analysisContext{
		currentPath:   ctx.currentPath + "/" + segment,
		dynamicParams: append([]ParamInfo{}, ctx.dynamicParams...),
		tagName:       ctx.tagName,
		mapKeyCount:   ctx.mapKeyCount,
		sliceIdxCount: ctx.sliceIdxCount,
	}
	return newCtx
}

func (ctx *analysisContext) withMapKey() *analysisContext {
	paramName := "key"
	if ctx.mapKeyCount > 0 {
		paramName = "key" + string(rune('0'+ctx.mapKeyCount+1))
	}
	newCtx := &analysisContext{
		currentPath:   ctx.currentPath + "/{" + paramName + "}",
		dynamicParams: append(append([]ParamInfo{}, ctx.dynamicParams...), ParamInfo{Name: paramName, Type: "string"}),
		tagName:       ctx.tagName,
		mapKeyCount:   ctx.mapKeyCount + 1,
		sliceIdxCount: ctx.sliceIdxCount,
	}
	return newCtx
}

func (ctx *analysisContext) withSliceIndex() *analysisContext {
	paramName := "index"
	if ctx.sliceIdxCount > 0 {
		paramName = "index" + string(rune('0'+ctx.sliceIdxCount+1))
	}
	newCtx := &analysisContext{
		currentPath:   ctx.currentPath + "/{" + paramName + "}",
		dynamicParams: append(append([]ParamInfo{}, ctx.dynamicParams...), ParamInfo{Name: paramName, Type: "int"}),
		tagName:       ctx.tagName,
		mapKeyCount:   ctx.mapKeyCount,
		sliceIdxCount: ctx.sliceIdxCount + 1,
	}
	return newCtx
}

func (ctx *analysisContext) isDynamic() bool {
	return len(ctx.dynamicParams) > 0
}

// analyzeStruct analyzes a struct type and returns all path information.
func analyzeStruct(structType *types.Struct, tagName string) (*AnalysisResult, error) {
	result := &AnalysisResult{
		Paths: make([]PathInfo, 0),
	}

	ctx := newAnalysisContext(tagName)
	analyzeStructRecursive(structType, ctx, result)

	return result, nil
}

func analyzeStructRecursive(structType *types.Struct, ctx *analysisContext, result *AnalysisResult) {
	for i := 0; i < structType.NumFields(); i++ {
		field := structType.Field(i)

		// Skip unexported fields
		if !field.Exported() {
			continue
		}

		tag := structType.Tag(i)
		fieldKey := getFieldKey(field.Name(), tag, ctx.tagName)
		if fieldKey == "-" {
			continue
		}

		// Check for jubako tag
		jubakoPath, isAbsolute := parseJubakoTagPath(tag)

		// Determine the effective path
		var effectivePath string
		var effectiveCtx *analysisContext

		if jubakoPath != "" && isAbsolute {
			// Absolute path - use as-is, ignore current context
			effectivePath = jubakoPath
			effectiveCtx = &analysisContext{
				currentPath:   jubakoPath,
				dynamicParams: nil, // Absolute paths reset dynamic params
				tagName:       ctx.tagName,
			}
		} else if jubakoPath != "" {
			// Relative jubako path
			effectivePath = ctx.currentPath + jubakoPath
			effectiveCtx = ctx.withPath(strings.TrimPrefix(jubakoPath, "/"))
			effectiveCtx.currentPath = effectivePath
		} else {
			// Use field key
			effectivePath = ctx.currentPath + "/" + fieldKey
			effectiveCtx = ctx.withPath(fieldKey)
		}

		// Analyze field type
		fieldType := field.Type()
		analyzeFieldType(fieldType, field.Name(), effectivePath, effectiveCtx, result)
	}
}

func analyzeFieldType(fieldType types.Type, fieldName, path string, ctx *analysisContext, result *AnalysisResult) {
	// Handle pointer types
	if ptr, ok := fieldType.(*types.Pointer); ok {
		fieldType = ptr.Elem()
	}

	// Handle named types - check if it's an external package type
	if named, ok := fieldType.(*types.Named); ok {
		// External package types (like time.Time) should be treated as leaf values
		if isExternalType(named) {
			addPathInfo(path, fieldName, ctx, result)
			return
		}
		fieldType = named.Underlying()
	}

	switch t := fieldType.(type) {
	case *types.Struct:
		// Recurse into nested struct
		analyzeStructRecursive(t, ctx, result)

	case *types.Slice, *types.Array:
		// Always add path to the container itself
		addPathInfo(path, fieldName, ctx, result)

		var elemType types.Type
		if slice, ok := t.(*types.Slice); ok {
			elemType = slice.Elem()
		} else if arr, ok := t.(*types.Array); ok {
			elemType = arr.Elem()
		}

		// Handle pointer element
		if ptr, ok := elemType.(*types.Pointer); ok {
			elemType = ptr.Elem()
		}

		// Handle named element - check for external types
		if named, ok := elemType.(*types.Named); ok {
			if isExternalType(named) {
				// External type elements don't need further recursion
				break
			}
			elemType = named.Underlying()
		}

		if structElem, ok := elemType.(*types.Struct); ok {
			// Slice of structs - add dynamic index and recurse
			sliceCtx := ctx.withSliceIndex()
			analyzeStructRecursive(structElem, sliceCtx, result)
		}

	case *types.Map:
		// Always add path to the container itself
		addPathInfo(path, fieldName, ctx, result)

		valueType := t.Elem()

		// Handle pointer value
		if ptr, ok := valueType.(*types.Pointer); ok {
			valueType = ptr.Elem()
		}

		// Handle named value - check for external types
		if named, ok := valueType.(*types.Named); ok {
			if isExternalType(named) {
				// External type values don't need further recursion
				break
			}
			valueType = named.Underlying()
		}

		if structValue, ok := valueType.(*types.Struct); ok {
			// Map with struct values - add dynamic key and recurse
			mapCtx := ctx.withMapKey()
			analyzeStructRecursive(structValue, mapCtx, result)
		}

	default:
		// Leaf value (primitives, strings, etc.)
		addPathInfo(path, fieldName, ctx, result)
	}
}

func addPathInfo(path, fieldName string, ctx *analysisContext, result *AnalysisResult) {
	info := PathInfo{
		JSONPointer:   path,
		FieldName:     fieldName,
		DynamicParams: append([]ParamInfo{}, ctx.dynamicParams...),
	}

	if ctx.isDynamic() {
		info.FuncName = generateFuncName(path)
		info.Comment = fmt.Sprintf("Path pattern: %s", path)
	} else {
		info.ConstName = generateConstName(path)
	}

	result.Paths = append(result.Paths, info)
}

// isExternalType checks if a named type is from an external package.
// External types (like time.Time, sql.NullString) should be treated as leaf values.
func isExternalType(named *types.Named) bool {
	obj := named.Obj()
	if obj == nil {
		return false
	}
	pkg := obj.Pkg()
	if pkg == nil {
		// Built-in types
		return false
	}
	// Standard library and external packages have package paths
	// that don't match the current package being analyzed.
	// For simplicity, we treat any named type with a package path as external.
	pkgPath := pkg.Path()
	// Common external packages that should be treated as leaf values
	return pkgPath != "" && (strings.HasPrefix(pkgPath, "time") ||
		strings.HasPrefix(pkgPath, "database/sql") ||
		strings.HasPrefix(pkgPath, "encoding/json") ||
		strings.HasPrefix(pkgPath, "net/") ||
		strings.HasPrefix(pkgPath, "crypto/") ||
		!isLocalPackage(pkgPath))
}

// isLocalPackage checks if a package path looks like a local/user package.
// This is a heuristic - packages without dots in the first segment are likely stdlib.
func isLocalPackage(pkgPath string) bool {
	// Standard library packages don't have dots in them
	// User packages typically have domain names (e.g., github.com/...)
	firstSegment := pkgPath
	if idx := strings.Index(pkgPath, "/"); idx > 0 {
		firstSegment = pkgPath[:idx]
	}
	return strings.Contains(firstSegment, ".")
}

// getFieldKey returns the field key from the tag or field name.
func getFieldKey(fieldName, tag, tagName string) string {
	structTag := reflect.StructTag(tag)
	tagValue := structTag.Get(tagName)
	if tagValue == "" {
		return fieldName
	}

	// Parse tag value (same as encoding/json: split by comma, use first part)
	if idx := strings.Index(tagValue, ","); idx >= 0 {
		tagValue = tagValue[:idx]
	}

	if tagValue == "" {
		return fieldName
	}

	return tagValue
}

// parseJubakoTagPath extracts the path from a jubako tag.
// Returns the path and whether it's absolute.
func parseJubakoTagPath(tag string) (path string, isAbsolute bool) {
	structTag := reflect.StructTag(tag)
	jubakoValue := structTag.Get(jubakoTagName)
	if jubakoValue == "" {
		return "", false
	}

	// Split by comma to separate path from directives
	parts := strings.Split(jubakoValue, ",")
	pathPart := strings.TrimSpace(parts[0])

	// Skip if it's just a directive
	switch pathPart {
	case "sensitive", "!sensitive", "-":
		return "", false
	}

	// Check for explicit relative prefix
	if strings.HasPrefix(pathPart, "./") {
		return "/" + pathPart[2:], false
	}

	// Absolute paths start with "/"
	if strings.HasPrefix(pathPart, "/") {
		return pathPart, true
	}

	// No leading "/" means relative path
	if pathPart != "" {
		return "/" + pathPart, false
	}

	return "", false
}

// generateConstName generates a constant name from a JSONPointer path.
func generateConstName(path string) string {
	segments := splitPath(path)
	var parts []string
	for _, seg := range segments {
		parts = append(parts, toCamelCase(seg))
	}
	return "Path" + strings.Join(parts, "")
}

// generateFuncName generates a function name from a JSONPointer path.
// Dynamic segments (e.g., {key}) are skipped.
func generateFuncName(path string) string {
	segments := splitPath(path)
	var parts []string
	for _, seg := range segments {
		// Skip dynamic segments
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			continue
		}
		parts = append(parts, toCamelCase(seg))
	}
	return "Path" + strings.Join(parts, "")
}

// splitPath splits a JSONPointer path into segments.
func splitPath(path string) []string {
	if path == "" || path == "/" {
		return nil
	}
	// Remove leading slash
	path = strings.TrimPrefix(path, "/")
	return strings.Split(path, "/")
}

// toCamelCase converts a string to CamelCase.
var separatorRegex = regexp.MustCompile(`[_\-\.]+`)

func toCamelCase(s string) string {
	// Handle common separators: _, -, .
	parts := separatorRegex.Split(s, -1)

	var result strings.Builder
	for _, part := range parts {
		if len(part) > 0 {
			runes := []rune(part)
			runes[0] = unicode.ToUpper(runes[0])
			result.WriteString(string(runes))
		}
	}

	return result.String()
}

