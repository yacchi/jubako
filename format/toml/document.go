// Package toml provides a TOML implementation of the document.Document interface.
//
// It preserves comments and formatting by performing minimal text edits on the
// original TOML bytes (using github.com/pelletier/go-toml/v2/unstable for parsing).
// When no modifications are performed, Marshal returns the input bytes verbatim.
package toml

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jsonptr"
	"github.com/yacchi/jubako/mapdoc"
)

// Document is a TOML document implementation backed by the raw TOML bytes.
type Document struct {
	src []byte
}

// Ensure Document implements document.Document interface.
var _ document.Document = (*Document)(nil)

// New creates a new empty TOML document.
func New() *Document {
	return &Document{src: nil}
}

// Parse parses TOML bytes into a Document.
//
// Empty/whitespace input is treated as an empty document.
func Parse(data []byte) (document.Document, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return New(), nil
	}
	// Validate by decoding once.
	var root map[string]any
	if err := toml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}
	return &Document{src: append([]byte(nil), data...)}, nil
}

// Format returns the document format.
func (d *Document) Format() document.DocumentFormat {
	return document.FormatTOML
}

// Marshal returns the current TOML bytes, preserving comments and formatting.
func (d *Document) Marshal() ([]byte, error) {
	return append([]byte(nil), d.src...), nil
}

// Get retrieves the value at the specified JSON Pointer path.
func (d *Document) Get(path string) (any, bool) {
	if path == "" || path == "/" {
		root, err := d.decodeRoot()
		if err != nil {
			return nil, false
		}
		return root, true
	}

	keys, err := jsonptr.Parse(path)
	if err != nil {
		return nil, false
	}

	root, err := d.decodeRoot()
	if err != nil {
		return nil, false
	}
	return mapdoc.Get(root, keys)
}

// Set sets the value at the specified JSON Pointer path, creating intermediate tables as needed.
func (d *Document) Set(path string, value any) error {
	keys, err := jsonptr.Parse(path)
	if err != nil {
		return &document.InvalidPathError{Path: path, Reason: err.Error()}
	}
	if len(keys) == 0 {
		return &document.InvalidPathError{Path: path, Reason: "cannot set root document"}
	}

	if containsNil(value) {
		return document.UnsupportedAt(path, "TOML does not support null values")
	}

	// If the path contains an array index segment, update the owning array value
	// and write it back as a single TOML value.
	if firstIdx := firstArrayIndex(keys); firstIdx >= 0 {
		if firstIdx == 0 {
			return &document.InvalidPathError{Path: path, Reason: "path cannot start with an array index"}
		}
		containerKeys := keys[:firstIdx]
		relativeKeys := keys[firstIdx:]

		root, err := d.decodeRoot()
		if err != nil {
			return err
		}

		// Ensure array exists at container path.
		if _, ok := getAny(root, containerKeys); !ok {
			setAny(root, containerKeys, []any{})
		}
		arrAny, ok := getAny(root, containerKeys)
		if !ok {
			return &document.PathNotFoundError{Path: path}
		}
		arr, ok := arrAny.([]any)
		if !ok {
			return &document.TypeMismatchError{Path: buildPointer(containerKeys), Expected: "array", Actual: fmt.Sprintf("%T", arrAny)}
		}

		updated, err := setAnyInArray(arr, relativeKeys, value)
		if err != nil {
			return err
		}
		setAny(root, containerKeys, updated)
		return d.setNonIndexed(containerKeys, updated)
	}

	return d.setNonIndexed(keys, value)
}

// Delete removes the value at the specified JSON Pointer path.
func (d *Document) Delete(path string) error {
	keys, err := jsonptr.Parse(path)
	if err != nil {
		return &document.InvalidPathError{Path: path, Reason: err.Error()}
	}
	if len(keys) == 0 {
		return &document.InvalidPathError{Path: path, Reason: "cannot delete root document"}
	}

	// Array element removal: update the owning array value and write back.
	if firstIdx := firstArrayIndex(keys); firstIdx >= 0 {
		if firstIdx == 0 {
			return &document.InvalidPathError{Path: path, Reason: "path cannot start with an array index"}
		}
		containerKeys := keys[:firstIdx]
		relativeKeys := keys[firstIdx:]

		root, err := d.decodeRoot()
		if err != nil {
			return err
		}
		arrAny, ok := getAny(root, containerKeys)
		if !ok {
			return nil
		}
		arr, ok := arrAny.([]any)
		if !ok {
			return &document.TypeMismatchError{Path: buildPointer(containerKeys), Expected: "array", Actual: fmt.Sprintf("%T", arrAny)}
		}

		updated, err := deleteAnyInArray(arr, relativeKeys)
		if err != nil {
			return err
		}
		setAny(root, containerKeys, updated)
		return d.setNonIndexed(containerKeys, updated)
	}

	return d.deleteNonIndexed(keys)
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
	// go-toml doesn't guarantee trailing newline; keep as-is.
	return b, nil
}

func (d *Document) decodeRoot() (map[string]any, error) {
	if len(bytes.TrimSpace(d.src)) == 0 {
		return map[string]any{}, nil
	}
	var root map[string]any
	if err := toml.Unmarshal(d.src, &root); err != nil {
		return nil, fmt.Errorf("failed to decode TOML: %w", err)
	}
	if root == nil {
		return map[string]any{}, nil
	}
	return root, nil
}

func (d *Document) setNonIndexed(keys []string, value any) error {
	if len(keys) == 0 {
		return &document.InvalidPathError{Path: "", Reason: "cannot set root document"}
	}
	for _, k := range keys {
		if isArrayIndex(k) {
			return &document.InvalidPathError{Path: buildPointer(keys), Reason: "internal error: indexed path passed to setNonIndexed"}
		}
	}

	tablePath := keys[:len(keys)-1]
	leafKey := keys[len(keys)-1]

	newValue, err := formatTOMLValue(value)
	if err != nil {
		return err
	}

	idx, err := buildIndex(d.src)
	if err != nil {
		return err
	}

	fullPath := append(append([]string(nil), tablePath...), leafKey)
	if kv, ok := idx.kvByPath[strings.Join(fullPath, "\x00")]; ok {
		d.src = replaceBytes(d.src, kv.valueStart, kv.valueEnd, []byte(newValue))
		return nil
	}

	insertPos, err := idx.ensureSectionEnd(tablePath, &d.src)
	if err != nil {
		return err
	}

	line := []byte(formatKey(leafKey) + " = " + newValue + "\n")
	d.src = insertBytes(d.src, insertPos, ensureLeadingNewline(d.src, insertPos, line))
	return nil
}

func (d *Document) deleteNonIndexed(keys []string) error {
	if len(keys) == 0 {
		return &document.InvalidPathError{Path: "", Reason: "cannot delete root document"}
	}
	for _, k := range keys {
		if isArrayIndex(k) {
			return &document.InvalidPathError{Path: buildPointer(keys), Reason: "internal error: indexed path passed to deleteNonIndexed"}
		}
	}

	tablePath := keys[:len(keys)-1]
	leafKey := keys[len(keys)-1]
	fullPath := append(append([]string(nil), tablePath...), leafKey)

	idx, err := buildIndex(d.src)
	if err != nil {
		return err
	}

	kv, ok := idx.kvByPath[strings.Join(fullPath, "\x00")]
	if !ok {
		return nil
	}
	d.src = replaceBytes(d.src, kv.lineStart, kv.lineEnd, nil)
	return nil
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
			tpath, off, err := tablePathAndLineStart(&p, n)
			if err != nil {
				return nil, err
			}
			currentTable = tpath
			idx.sections = append(idx.sections, sectionInfo{path: tpath, lineStart: off})
		case unstable.ArrayTable:
			// Array tables are not currently supported as a map-like container for JSON pointers.
			tpath, off, err := tablePathAndLineStart(&p, n)
			if err != nil {
				return nil, err
			}
			currentTable = tpath
			idx.sections = append(idx.sections, sectionInfo{path: tpath, lineStart: off})
		case unstable.KeyValue:
			kv, fullPath, err := keyValueInfo(&p, n, currentTable, src)
			if err != nil {
				return nil, err
			}
			idx.kvByPath[strings.Join(fullPath, "\x00")] = kv
		}
	}
	if err := p.Error(); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	// Compute section end offsets.
	// Root section is implicit: start at 0, ends at first header (or EOF).
	// Each section ends at the start of the next header (or EOF).
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

func (idx *index) ensureSectionEnd(tablePath []string, src *[]byte) (int, error) {
	if len(tablePath) == 0 {
		// Insert in root section, before the first header if present.
		firstHeader := len(*src)
		for _, s := range idx.sections {
			if s.lineStart < firstHeader {
				firstHeader = s.lineStart
			}
		}
		return firstHeader, nil
	}

	for _, s := range idx.sections {
		if equalStringSlice(s.path, tablePath) {
			return s.lineEnd, nil
		}
	}

	// Append a new table header at EOF.
	if len(*src) > 0 && (*src)[len(*src)-1] != '\n' {
		*src = append(*src, '\n')
	}
	header := "[" + formatDottedKey(tablePath) + "]\n"
	*src = append(*src, []byte(header)...)
	return len(*src), nil
}

func tablePathAndLineStart(p *unstable.Parser, n *unstable.Node) ([]string, int, error) {
	var parts []string
	it := n.Key()
	var firstOff int = -1
	for it.Next() {
		k := it.Node()
		parts = append(parts, string(k.Data))
		if firstOff < 0 {
			firstOff = int(k.Raw.Offset)
		}
	}
	if firstOff < 0 {
		// Fallback: start at 0.
		firstOff = 0
	}
	lineStart := findLineStart(p.Data(), firstOff)
	return parts, lineStart, nil
}

func keyValueInfo(p *unstable.Parser, n *unstable.Node, currentTable []string, src []byte) (kvInfo, []string, error) {
	// KeyValue children: Value node, then Key nodes (possibly dotted).
	val := n.Value()
	if !val.Valid() {
		return kvInfo{}, nil, fmt.Errorf("invalid TOML AST: KeyValue without value")
	}

	// Value start offset.
	valueStart := rangeOffset(p, val)
	lineStart := findLineStart(src, valueStart)
	lineEnd := findLineEnd(src, valueStart)

	fullPath := append([]string(nil), currentTable...)
	it := n.Key()
	for it.Next() {
		fullPath = append(fullPath, string(it.Node().Data))
	}
	if len(fullPath) == 0 {
		return kvInfo{}, nil, fmt.Errorf("invalid TOML AST: KeyValue without key")
	}

	// If there is a trailing comment on this expression, we stop replacement before it.
	valueEnd := lineEnd
	next := n.Next()
	if next != nil && next.Kind == unstable.Comment && next.Raw.Length > 0 {
		commentOff := int(next.Raw.Offset)
		if commentOff > valueStart && commentOff <= lineEnd {
			valueEnd = commentOff
		}
	}

	// Trim trailing whitespace from replacement region so we don't leave double spaces.
	for valueEnd > valueStart && (src[valueEnd-1] == ' ' || src[valueEnd-1] == '\t') {
		valueEnd--
	}

	return kvInfo{
		lineStart:  lineStart,
		lineEnd:    lineEnd,
		valueStart: valueStart,
		valueEnd:   valueEnd,
	}, fullPath, nil
}

func rangeOffset(p *unstable.Parser, n *unstable.Node) int {
	if n.Raw.Length > 0 {
		return int(n.Raw.Offset)
	}
	if len(n.Data) > 0 {
		r := p.Range(n.Data)
		return int(r.Offset)
	}
	// Fallback: use the line start of the current parser left position.
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
	b, err := toml.Marshal(map[string]any{"__jubako__": v})
	if err != nil {
		return "", fmt.Errorf("failed to encode TOML value: %w", err)
	}
	// Extract after '=' on the first line.
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
	// Use bare key if possible.
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
	// Fallback to basic string key.
	// Reuse TOML encoder for correctness.
	s, err := formatTOMLValue(k)
	if err != nil {
		// Last resort: quote with double quotes, escaping quotes.
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
	cur := any(root)
	for _, k := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok := m[k]
		if !ok {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

func setAny(root map[string]any, keys []string, value any) {
	if len(keys) == 0 {
		return
	}
	cur := root
	for _, k := range keys[:len(keys)-1] {
		next, ok := cur[k].(map[string]any)
		if !ok {
			next = make(map[string]any)
			cur[k] = next
		}
		cur = next
	}
	cur[keys[len(keys)-1]] = value
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
		updated, err := deleteAnyInArray(nested, rest)
		if err != nil {
			return nil, err
		}
		arr[idx] = updated
		return arr, nil
	}

	nested, ok := elem.(map[string]any)
	if !ok {
		return arr, nil
	}
	deleteAny(nested, rest)
	arr[idx] = nested
	return arr, nil
}

func deleteAny(root map[string]any, keys []string) {
	if len(keys) == 0 {
		return
	}
	cur := root
	for _, k := range keys[:len(keys)-1] {
		next, ok := cur[k].(map[string]any)
		if !ok {
			return
		}
		cur = next
	}
	delete(cur, keys[len(keys)-1])
}
