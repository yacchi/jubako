// Package toml provides a TOML implementation of the document.Document interface.
//
// It preserves comments and formatting by performing minimal text edits on the
// original TOML bytes (using github.com/pelletier/go-toml/v2/unstable for parsing).
// When no modifications are performed, Apply returns the input bytes verbatim.
package toml

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jsonptr"
)

// Document is a TOML document implementation.
// It is stateless - parsing and serialization happen on demand.
type Document struct{}

// Ensure Document implements document.Document interface.
var _ document.Document = (*Document)(nil)

var tomlMarshal = toml.Marshal
var tomlUnmarshal = toml.Unmarshal

// New returns a TOML Document.
//
// Example:
//
//	src := fs.New("~/.config/app.toml")
//	layer.New("user", src, toml.New())
func New() *Document {
	return &Document{}
}

// Format returns the document format.
func (d *Document) Format() document.DocumentFormat {
	return document.FormatTOML
}

// Get parses data bytes and returns content as map[string]any.
// Returns empty map if data is nil or empty.
func (d *Document) Get(data []byte) (map[string]any, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}

	var result map[string]any
	if err := tomlUnmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	if result == nil {
		return map[string]any{}, nil
	}

	return result, nil
}

// Apply applies changeset to data bytes and returns new bytes.
// If changeset is provided: parses data, applies changeset operations
// using minimal text edits to preserve comments, then returns the result.
// If changeset is empty: marshals parsed data directly.
func (d *Document) Apply(data []byte, changeset document.JSONPatchSet) ([]byte, error) {
	// Parse data to check for nil values
	var current map[string]any
	if len(bytes.TrimSpace(data)) > 0 {
		if err := tomlUnmarshal(data, &current); err != nil {
			return nil, fmt.Errorf("failed to parse TOML: %w", err)
		}
	}
	if current == nil {
		current = map[string]any{}
	}

	// Check for nil values in current (TOML doesn't support null)
	if err := checkNilMap("", current); err != nil {
		return nil, err
	}

	// If no changeset, just marshal current data directly
	if changeset.IsEmpty() {
		return toml.Marshal(current)
	}

	// Use original data as source for comment preservation
	src := data
	if len(bytes.TrimSpace(src)) == 0 {
		// No existing data, just marshal after applying patches in-memory
		for _, patch := range changeset {
			keys, err := jsonptr.Parse(patch.Path)
			if err != nil || len(keys) == 0 {
				continue
			}
			switch patch.Op {
			case document.PatchOpAdd, document.PatchOpReplace:
				if containsNil(patch.Value) {
					continue
				}
				jsonptr.SetPath(current, patch.Path, patch.Value)
			case document.PatchOpRemove:
				jsonptr.DeletePath(current, patch.Path)
			}
		}
		return toml.Marshal(current)
	}

	// Apply each patch operation
	var err error
	for _, patch := range changeset {
		keys, parseErr := jsonptr.Parse(patch.Path)
		if parseErr != nil {
			continue // Skip invalid paths
		}

		if len(keys) == 0 {
			continue // Skip root path operations
		}

		switch patch.Op {
		case document.PatchOpAdd, document.PatchOpReplace:
			if containsNil(patch.Value) {
				continue // TOML doesn't support null
			}
			src, err = applySet(src, keys, patch.Value)
			if err != nil {
				// If operation fails, continue with remaining patches
				continue
			}
		case document.PatchOpRemove:
			src, err = applyDelete(src, keys)
			if err != nil {
				continue
			}
		}
	}

	return src, nil
}

// MarshalTestData generates TOML bytes (without comment preservation requirements) for testing.
func (d *Document) MarshalTestData(data map[string]any) ([]byte, error) {
	if err := checkNilMap("", data); err != nil {
		return nil, err
	}
	b, err := toml.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal TOML test data: %w", err)
	}
	return b, nil
}

// applySet applies a set operation to the source bytes.
func applySet(src []byte, keys []string, value any) ([]byte, error) {
	for _, k := range keys {
		if isArrayIndex(k) {
			return applySetWithArray(src, keys, value)
		}
	}

	return applySetNonIndexed(src, keys, value)
}

// applySetWithArray handles set operations that involve array indices.
func applySetWithArray(src []byte, keys []string, value any) ([]byte, error) {
	firstIdx := firstArrayIndex(keys)
	if firstIdx < 0 {
		return applySetNonIndexed(src, keys, value)
	}
	if firstIdx == 0 {
		return nil, &document.InvalidPathError{Path: buildPointer(keys), Reason: "path cannot start with an array index"}
	}

	containerKeys := keys[:firstIdx]
	relativeKeys := keys[firstIdx:]

	// Decode current data
	var root map[string]any
	if len(bytes.TrimSpace(src)) > 0 {
		if err := tomlUnmarshal(src, &root); err != nil {
			return nil, err
		}
	}
	if root == nil {
		root = make(map[string]any)
	}

	// Ensure array exists at container path
	arrAny, ok := getAny(root, containerKeys)
	if !ok {
		arrAny = []any{}
		setAny(root, containerKeys, arrAny)
	}
	arr, ok := arrAny.([]any)
	if !ok {
		return nil, &document.TypeMismatchError{Path: buildPointer(containerKeys), Expected: "array", Actual: fmt.Sprintf("%T", arrAny)}
	}

	updated, err := setAnyInArray(arr, relativeKeys, value)
	if err != nil {
		return nil, err
	}
	setAny(root, containerKeys, updated)

	return applySetNonIndexed(src, containerKeys, updated)
}

// applySetNonIndexed applies a set operation without array indices.
func applySetNonIndexed(src []byte, keys []string, value any) ([]byte, error) {
	if len(keys) == 0 {
		return nil, &document.InvalidPathError{Path: "", Reason: "cannot set root document"}
	}

	tablePath := keys[:len(keys)-1]
	leafKey := keys[len(keys)-1]

	newValue, err := formatTOMLValue(value)
	if err != nil {
		return nil, err
	}

	idx, err := buildIndex(src)
	if err != nil {
		return nil, err
	}

	fullPath := append(append([]string(nil), tablePath...), leafKey)
	if kv, ok := idx.kvByPath[strings.Join(fullPath, "\x00")]; ok {
		return replaceBytes(src, kv.valueStart, kv.valueEnd, []byte(newValue)), nil
	}

	insertPos := idx.ensureSectionEnd(tablePath, &src)

	line := []byte(formatKey(leafKey) + " = " + newValue + "\n")
	return insertBytes(src, insertPos, ensureLeadingNewline(src, insertPos, line)), nil
}

// applyDelete applies a delete operation to the source bytes.
func applyDelete(src []byte, keys []string) ([]byte, error) {
	for _, k := range keys {
		if isArrayIndex(k) {
			return applyDeleteWithArray(src, keys)
		}
	}

	return applyDeleteNonIndexed(src, keys)
}

// applyDeleteWithArray handles delete operations that involve array indices.
func applyDeleteWithArray(src []byte, keys []string) ([]byte, error) {
	firstIdx := firstArrayIndex(keys)
	if firstIdx < 0 {
		return applyDeleteNonIndexed(src, keys)
	}
	if firstIdx == 0 {
		return nil, &document.InvalidPathError{Path: buildPointer(keys), Reason: "path cannot start with an array index"}
	}

	containerKeys := keys[:firstIdx]
	relativeKeys := keys[firstIdx:]

	// Decode current data
	var root map[string]any
	if len(bytes.TrimSpace(src)) > 0 {
		if err := tomlUnmarshal(src, &root); err != nil {
			return nil, err
		}
	}
	if root == nil {
		return src, nil
	}

	arrAny, ok := getAny(root, containerKeys)
	if !ok {
		return src, nil
	}
	arr, ok := arrAny.([]any)
	if !ok {
		return nil, &document.TypeMismatchError{Path: buildPointer(containerKeys), Expected: "array", Actual: fmt.Sprintf("%T", arrAny)}
	}

	updated, _ := deleteAnyInArray(arr, relativeKeys)
	setAny(root, containerKeys, updated)

	return applySetNonIndexed(src, containerKeys, updated)
}

// applyDeleteNonIndexed applies a delete operation without array indices.
func applyDeleteNonIndexed(src []byte, keys []string) ([]byte, error) {
	if len(keys) == 0 {
		return nil, &document.InvalidPathError{Path: "", Reason: "cannot delete root document"}
	}

	tablePath := keys[:len(keys)-1]
	leafKey := keys[len(keys)-1]
	fullPath := append(append([]string(nil), tablePath...), leafKey)

	idx, err := buildIndex(src)
	if err != nil {
		return nil, err
	}

	kv, ok := idx.kvByPath[strings.Join(fullPath, "\x00")]
	if !ok {
		return src, nil
	}
	return replaceBytes(src, kv.lineStart, kv.lineEnd, nil), nil
}

type kvInfo struct {
	lineStart  int
	lineEnd    int
	valueStart int
	valueEnd   int
}

type sectionInfo struct {
	path      []string
	lineStart int
	lineEnd   int
}

type index struct {
	sections []sectionInfo
	kvByPath map[string]kvInfo
}

func buildIndex(src []byte) (*index, error) {
	idx := &index{
		sections: make([]sectionInfo, 0),
		kvByPath: make(map[string]kvInfo),
	}

	p := unstable.Parser{KeepComments: true}
	p.Reset(src)

	currentTable := []string(nil)
	for p.NextExpression() {
		n := p.Expression()
		switch n.Kind {
		case unstable.Table:
			tpath, off := tablePathAndLineStart(&p, n)
			currentTable = tpath
			idx.sections = append(idx.sections, sectionInfo{path: tpath, lineStart: off})
		case unstable.ArrayTable:
			// Array tables are not currently supported as a map-like container for JSON pointers.
			tpath, off := tablePathAndLineStart(&p, n)
			currentTable = tpath
			idx.sections = append(idx.sections, sectionInfo{path: tpath, lineStart: off})
		case unstable.KeyValue:
			kv, fullPath := keyValueInfo(&p, n, currentTable, src)
			idx.kvByPath[strings.Join(fullPath, "\x00")] = kv
		}
	}
	if err := p.Error(); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	// Compute section end offsets.
	for i := range idx.sections {
		start := idx.sections[i].lineStart
		end := len(src)
		for j := i + 1; j < len(idx.sections); j++ {
			if idx.sections[j].lineStart > start {
				end = idx.sections[j].lineStart
				break
			}
		}
		idx.sections[i].lineEnd = end
	}
	return idx, nil
}

func (idx *index) ensureSectionEnd(tablePath []string, src *[]byte) int {
	if len(tablePath) == 0 {
		// Insert in root section, before the first header if present.
		firstHeader := len(*src)
		for _, s := range idx.sections {
			if s.lineStart < firstHeader {
				firstHeader = s.lineStart
			}
		}
		return firstHeader
	}

	for _, s := range idx.sections {
		if equalStringSlice(s.path, tablePath) {
			return s.lineEnd
		}
	}

	// Append a new table header at EOF.
	if len(*src) > 0 && (*src)[len(*src)-1] != '\n' {
		*src = append(*src, '\n')
	}
	header := "[" + formatDottedKey(tablePath) + "]\n"
	*src = append(*src, []byte(header)...)
	return len(*src)
}

func tablePathAndLineStart(p *unstable.Parser, n *unstable.Node) ([]string, int) {
	var parts []string
	it := n.Key()
	firstOff := 0
	firstKey := true
	for it.Next() {
		k := it.Node()
		parts = append(parts, string(k.Data))
		if firstKey {
			firstOff = int(k.Raw.Offset)
			firstKey = false
		}
	}

	lineStart := findLineStart(p.Data(), firstOff)
	return parts, lineStart
}

func keyValueInfo(p *unstable.Parser, n *unstable.Node, currentTable []string, src []byte) (kvInfo, []string) {
	val := n.Value()
	fullPath := append([]string(nil), currentTable...)
	it := n.Key()
	keyOff := int(n.Raw.Offset)
	firstKey := true
	for it.Next() {
		k := it.Node()
		fullPath = append(fullPath, string(k.Data))
		if firstKey {
			keyOff = int(k.Raw.Offset)
			firstKey = false
		}
	}

	lineStart := findLineStart(src, keyOff)
	lineEnd := findLineEnd(src, keyOff)

	// Prefer the value node offset, but fall back to scanning the line.
	valueStart := rangeOffset(p, val)
	if valueStart <= lineStart || valueStart >= lineEnd {
		eq := bytes.IndexByte(src[lineStart:lineEnd], '=')
		valueStart = lineStart + eq + 1
		for valueStart < lineEnd && (src[valueStart] == ' ' || src[valueStart] == '\t') {
			valueStart++
		}
	}

	valueEnd := lineEnd
	next := n.Next()
	if next != nil && next.Kind == unstable.Comment && next.Raw.Length > 0 {
		commentOff := int(next.Raw.Offset)
		if commentOff > valueStart && commentOff <= lineEnd {
			valueEnd = commentOff
		}
	}

	for valueEnd > valueStart && (src[valueEnd-1] == ' ' || src[valueEnd-1] == '\t' || src[valueEnd-1] == '\n' || src[valueEnd-1] == '\r') {
		valueEnd--
	}

	return kvInfo{
		lineStart:  lineStart,
		lineEnd:    lineEnd,
		valueStart: valueStart,
		valueEnd:   valueEnd,
	}, fullPath
}

func rangeOffset(p *unstable.Parser, n *unstable.Node) int {
	// Prefer Raw offsets, which typically include the full token span (e.g. quotes for strings).
	// Some nodes (e.g. empty arrays) may not carry reliable Raw spans; callers should fall back.
	if n.Raw.Length > 0 || n.Raw.Offset > 0 {
		return int(n.Raw.Offset)
	}
	if len(n.Data) > 0 {
		r := p.Range(n.Data)
		return int(r.Offset)
	}
	return 0
}

func findLineStart(b []byte, off int) int {
	if off <= 0 {
		return 0
	}
	for i := off - 1; i >= 0; i-- {
		if b[i] == '\n' {
			return i + 1
		}
	}
	return 0
}

func findLineEnd(b []byte, off int) int {
	for i := off; i < len(b); i++ {
		if b[i] == '\n' {
			return i + 1
		}
	}
	return len(b)
}

func replaceBytes(src []byte, start, end int, replacement []byte) []byte {
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	out := make([]byte, 0, len(src)-(end-start)+len(replacement))
	out = append(out, src[:start]...)
	out = append(out, replacement...)
	out = append(out, src[end:]...)
	return out
}

func insertBytes(src []byte, pos int, data []byte) []byte {
	if pos < 0 {
		pos = 0
	}
	if pos > len(src) {
		pos = len(src)
	}
	out := make([]byte, 0, len(src)+len(data))
	out = append(out, src[:pos]...)
	out = append(out, data...)
	out = append(out, src[pos:]...)
	return out
}

func ensureLeadingNewline(src []byte, pos int, line []byte) []byte {
	if pos == 0 {
		return line
	}
	if pos > 0 && src[pos-1] != '\n' {
		return append([]byte{'\n'}, line...)
	}
	return line
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func isArrayIndex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func containsNil(v any) bool {
	if v == nil {
		return true
	}
	switch t := v.(type) {
	case map[string]any:
		for _, vv := range t {
			if containsNil(vv) {
				return true
			}
		}
	case []any:
		for _, vv := range t {
			if containsNil(vv) {
				return true
			}
		}
	}
	return false
}

func checkNilMap(path string, m map[string]any) error {
	for k, v := range m {
		p := path + "/" + jsonptr.Escape(k)
		switch vv := v.(type) {
		case nil:
			return document.UnsupportedAt(p, "TOML does not support null values")
		case map[string]any:
			if err := checkNilMap(p, vv); err != nil {
				return err
			}
		case []any:
			if err := checkNilSlice(p, vv); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkNilSlice(path string, s []any) error {
	for i, v := range s {
		p := fmt.Sprintf("%s/%d", path, i)
		switch vv := v.(type) {
		case nil:
			return document.UnsupportedAt(p, "TOML does not support null values")
		case map[string]any:
			if err := checkNilMap(p, vv); err != nil {
				return err
			}
		case []any:
			if err := checkNilSlice(p, vv); err != nil {
				return err
			}
		}
	}
	return nil
}

func formatTOMLValue(v any) (string, error) {
	if v == nil {
		return "", document.Unsupported("TOML does not support null values")
	}
	b, err := tomlMarshal(map[string]any{"__jubako__": v})
	if err != nil {
		return "", fmt.Errorf("failed to encode TOML value: %w", err)
	}
	lineEnd := bytes.IndexByte(b, '\n')
	if lineEnd < 0 {
		lineEnd = len(b)
	}
	line := string(b[:lineEnd])
	eq := strings.IndexByte(line, '=')
	if eq < 0 {
		return "", fmt.Errorf("failed to encode TOML value")
	}
	return strings.TrimSpace(line[eq+1:]), nil
}

func formatDottedKey(keys []string) string {
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, formatKey(k))
	}
	return strings.Join(parts, ".")
}

func formatKey(k string) string {
	isBare := k != ""
	for i := 0; i < len(k); i++ {
		c := k[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			isBare = false
			break
		}
	}
	if isBare {
		return k
	}
	s, err := formatTOMLValue(k)
	if err != nil {
		return `"` + strings.ReplaceAll(k, `"`, `\"`) + `"`
	}
	return s
}

func buildPointer(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	var b bytes.Buffer
	for _, k := range keys {
		b.WriteByte('/')
		b.WriteString(jsonptr.Escape(k))
	}
	return b.String()
}

func firstArrayIndex(keys []string) int {
	for i, k := range keys {
		if isArrayIndex(k) {
			return i
		}
	}
	return -1
}

func parseArrayIndex(s string) (int, error) {
	if !isArrayIndex(s) {
		return 0, fmt.Errorf("invalid array index: %q", s)
	}
	n := 0
	for i := 0; i < len(s); i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n, nil
}

func getAny(root map[string]any, keys []string) (any, bool) {
	return jsonptr.GetByKeys(root, keys)
}

func setAny(root map[string]any, keys []string, value any) {
	jsonptr.SetByKeys(root, keys, value)
}

func setAnyInArray(arr []any, keys []string, value any) ([]any, error) {
	if len(keys) == 0 {
		return arr, fmt.Errorf("invalid array path")
	}
	idx, err := parseArrayIndex(keys[0])
	if err != nil {
		return nil, &document.InvalidPathError{Path: buildPointer(keys), Reason: err.Error()}
	}

	if idx > len(arr) {
		return nil, document.UnsupportedAt(buildPointer(keys), "TOML arrays cannot be extended with gaps")
	}
	if idx == len(arr) {
		arr = append(arr, nil)
	}

	if len(keys) == 1 {
		arr[idx] = value
		return arr, nil
	}

	rest := keys[1:]
	nextKey := rest[0]
	elem := arr[idx]

	if isArrayIndex(nextKey) {
		var nested []any
		if elem != nil {
			if a, ok := elem.([]any); ok {
				nested = a
			} else {
				return nil, &document.TypeMismatchError{Path: buildPointer(keys[:1]), Expected: "array", Actual: fmt.Sprintf("%T", elem)}
			}
		} else {
			nested = make([]any, 0)
		}
		updated, err := setAnyInArray(nested, rest, value)
		if err != nil {
			return nil, err
		}
		arr[idx] = updated
		return arr, nil
	}

	var nested map[string]any
	if elem != nil {
		if m, ok := elem.(map[string]any); ok {
			nested = m
		} else {
			return nil, &document.TypeMismatchError{Path: buildPointer(keys[:1]), Expected: "object", Actual: fmt.Sprintf("%T", elem)}
		}
	} else {
		nested = make(map[string]any)
	}
	setAny(nested, rest, value)
	arr[idx] = nested
	return arr, nil
}

func deleteAnyInArray(arr []any, keys []string) ([]any, error) {
	if len(keys) == 0 {
		return arr, fmt.Errorf("invalid array path")
	}
	idx, err := parseArrayIndex(keys[0])
	if err != nil {
		return nil, &document.InvalidPathError{Path: buildPointer(keys), Reason: err.Error()}
	}
	if idx < 0 || idx >= len(arr) {
		return arr, nil
	}
	if len(keys) == 1 {
		copy(arr[idx:], arr[idx+1:])
		arr = arr[:len(arr)-1]
		return arr, nil
	}

	rest := keys[1:]
	nextKey := rest[0]
	elem := arr[idx]

	if isArrayIndex(nextKey) {
		nested, ok := elem.([]any)
		if !ok {
			return arr, nil
		}
		updated, _ := deleteAnyInArray(nested, rest)
		arr[idx] = updated
		return arr, nil
	}

	nested, ok := elem.(map[string]any)
	if !ok {
		return arr, nil
	}
	jsonptr.DeleteByKeys(nested, rest)
	arr[idx] = nested
	return arr, nil
}
