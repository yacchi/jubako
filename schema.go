package jubako

import (
	"strings"

	"github.com/yacchi/jubako/jsonptr"
)

// Schema is a root-level container that holds both the hierarchical MappingTable
// (for transformation) and the flat MappingTrie (for path-based lookups).
//
// This design separates concerns:
//   - MappingTable: nested structure used by applyMappings for value transformation
//   - MappingTrie: flat trie structure for efficient path-based attribute lookups
//
// The Trie stores references to the same PathMapping instances as the Table,
// minimizing memory overhead while enabling O(path depth) lookups.
type Schema struct {
	// Table holds the hierarchical mapping structure for transformation.
	Table *MappingTable

	// Trie provides path-based lookup for field attributes.
	// Built from Table, indexed by source paths (JSONPointer format).
	Trie *MappingTrie
}

// NewSchema creates a Schema from a MappingTable.
// It builds the MappingTrie from the table for path-based lookups.
// Returns an empty Schema (with nil Table and Trie) if table is nil,
// ensuring that callers can safely access Schema.Trie without nil checks.
func NewSchema(table *MappingTable) *Schema {
	if table == nil {
		return &Schema{}
	}
	return &Schema{
		Table: table,
		Trie:  NewMappingTrie(table),
	}
}

// String returns a human-readable representation of the schema.
func (s *Schema) String() string {
	if s == nil {
		return "(nil schema)"
	}
	var sb strings.Builder
	sb.WriteString("Schema:\n")
	sb.WriteString("  Table:\n")
	if s.Table != nil {
		tableStr := s.Table.String()
		for _, line := range strings.Split(tableStr, "\n") {
			if line != "" {
				sb.WriteString("    ")
				sb.WriteString(line)
				sb.WriteString("\n")
			}
		}
	} else {
		sb.WriteString("    (nil)\n")
	}
	sb.WriteString("  Trie:\n")
	if s.Trie != nil {
		trieStr := s.Trie.String()
		for _, line := range strings.Split(trieStr, "\n") {
			if line != "" {
				sb.WriteString("    ")
				sb.WriteString(line)
				sb.WriteString("\n")
			}
		}
	} else {
		sb.WriteString("    (nil)\n")
	}
	return sb.String()
}

// MappingTrie is a trie structure for efficient path-based lookups of PathMapping.
// It is built from MappingTable and indexed by source paths (JSONPointer format).
//
// The trie supports:
//   - Exact path matching (e.g., "/credentials/password")
//   - Wildcard matching for slice indices (e.g., "/items/*/secret" matches "/items/0/secret")
//   - Wildcard matching for map keys (e.g., "/configs/*/apiKey" matches "/configs/prod/apiKey")
//
// Each leaf node stores a reference to the PathMapping, enabling lookup of any
// field attribute (Sensitive, SourcePath, etc.) without additional data structures.
type MappingTrie struct {
	root *mappingTrieNode
}

// mappingTrieNode represents a node in the mapping trie.
type mappingTrieNode struct {
	children map[string]*mappingTrieNode
	wildcard *mappingTrieNode  // child for wildcard matching (*)
	mapping  *PathMapping      // non-nil if this node represents a mapped field
}

// newMappingTrieNode creates a new trie node.
func newMappingTrieNode() *mappingTrieNode {
	return &mappingTrieNode{
		children: make(map[string]*mappingTrieNode),
	}
}

// NewMappingTrie creates a new MappingTrie from a MappingTable.
// It traverses the entire MappingTable structure and builds a trie
// indexed by source paths.
func NewMappingTrie(table *MappingTable) *MappingTrie {
	if table == nil {
		return nil
	}

	trie := &MappingTrie{
		root: newMappingTrieNode(),
	}

	// Build trie from mapping table
	trie.buildFromTable(table, "")

	return trie
}

// buildFromTable recursively builds the trie from a MappingTable.
func (t *MappingTrie) buildFromTable(table *MappingTable, prefix string) {
	if table == nil {
		return
	}

	// Add direct mappings
	for _, m := range table.Mappings {
		// Determine the actual path for trie insertion
		var path string
		if m.SourcePath != "" {
			// Use the source path from jubako tag
			// Note: SourcePath already has "/" prefix for JSONPointer format
			if m.IsRelative {
				// Relative path: prepend current prefix
				// SourcePath like "/secret" becomes prefix + "/secret"
				path = prefix + m.SourcePath
			} else {
				// Absolute path: use as-is
				path = m.SourcePath
			}
		} else {
			// No source path remapping: use structural path
			path = prefix + "/" + m.FieldKey
		}
		t.insert(path, m)
	}

	// Recurse into nested structs
	for key, nested := range table.Nested {
		nestedPrefix := prefix + "/" + key
		t.buildFromTable(nested, nestedPrefix)
	}

	// Handle slice elements with wildcard
	for key, elemTable := range table.SliceElement {
		// Use "*" as wildcard for slice indices
		elemPrefix := prefix + "/" + key + "/*"
		t.buildFromTable(elemTable, elemPrefix)
	}

	// Handle map values with wildcard
	for key, valueTable := range table.MapValue {
		// Use "*" as wildcard for map keys
		valuePrefix := prefix + "/" + key + "/*"
		t.buildFromTable(valueTable, valuePrefix)
	}
}

// insert adds a path with its mapping to the trie.
func (t *MappingTrie) insert(path string, mapping *PathMapping) {
	segments, err := jsonptr.Parse(path)
	if err != nil || len(segments) == 0 {
		return
	}

	node := t.root
	for _, seg := range segments {
		if seg == "*" {
			// Wildcard segment
			if node.wildcard == nil {
				node.wildcard = newMappingTrieNode()
			}
			node = node.wildcard
		} else {
			child, ok := node.children[seg]
			if !ok {
				child = newMappingTrieNode()
				node.children[seg] = child
			}
			node = child
		}
	}
	node.mapping = mapping
}

// Lookup returns the PathMapping for the given JSONPointer path.
// Returns nil if no mapping exists for the path.
// It supports wildcard matching for slice indices and map keys.
func (t *MappingTrie) Lookup(path string) *PathMapping {
	if t == nil || t.root == nil {
		return nil
	}

	segments, err := jsonptr.Parse(path)
	if err != nil || len(segments) == 0 {
		return nil
	}

	return t.lookupPath(t.root, segments, 0)
}

// lookupPath recursively matches path segments against the trie.
func (t *MappingTrie) lookupPath(node *mappingTrieNode, segments []string, idx int) *PathMapping {
	if node == nil {
		return nil
	}

	// Base case: we've matched all segments
	if idx >= len(segments) {
		return node.mapping
	}

	seg := segments[idx]

	// Try exact match first
	if child, ok := node.children[seg]; ok {
		if m := t.lookupPath(child, segments, idx+1); m != nil {
			return m
		}
	}

	// Try wildcard match (for slice indices and map keys)
	if node.wildcard != nil {
		if m := t.lookupPath(node.wildcard, segments, idx+1); m != nil {
			return m
		}
	}

	return nil
}

// IsSensitive checks if the given JSONPointer path is sensitive.
// This is a convenience method that checks the Sensitive attribute of the mapping.
func (t *MappingTrie) IsSensitive(path string) bool {
	m := t.Lookup(path)
	return m != nil && m.Sensitive == sensitiveExplicit
}

// IsEmpty returns true if the trie has no mappings.
func (t *MappingTrie) IsEmpty() bool {
	if t == nil || t.root == nil {
		return true
	}
	return t.isEmpty(t.root)
}

// isEmpty recursively checks if a node and its children have any mappings.
func (t *MappingTrie) isEmpty(node *mappingTrieNode) bool {
	if node.mapping != nil {
		return false
	}
	for _, child := range node.children {
		if !t.isEmpty(child) {
			return false
		}
	}
	if node.wildcard != nil && !t.isEmpty(node.wildcard) {
		return false
	}
	return true
}

// String returns a human-readable representation of the trie.
func (t *MappingTrie) String() string {
	if t == nil || t.root == nil {
		return "(empty)"
	}

	var sb strings.Builder
	sb.WriteString("MappingTrie:\n")
	t.writeNode(&sb, t.root, "  ", "")
	return sb.String()
}

// writeNode recursively writes a node and its children to the string builder.
func (t *MappingTrie) writeNode(sb *strings.Builder, node *mappingTrieNode, indent, path string) {
	if node.mapping != nil {
		sb.WriteString(indent)
		sb.WriteString(path)
		if node.mapping.Sensitive == sensitiveExplicit {
			sb.WriteString(" [sensitive]")
		}
		sb.WriteString("\n")
	}

	for key, child := range node.children {
		childPath := path + "/" + key
		t.writeNode(sb, child, indent, childPath)
	}

	if node.wildcard != nil {
		childPath := path + "/*"
		t.writeNode(sb, node.wildcard, indent, childPath)
	}
}
