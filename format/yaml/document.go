// Package yaml provides a YAML implementation of the document.Document interface
// and loaders for YAML configuration sources.
// It preserves comments and formatting by operating on yaml.Node AST.
package yaml

import (
	"fmt"
	"strconv"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jsonptr"
	"gopkg.in/yaml.v3"
)

// Document is a Document implementation for YAML format.
// It preserves comments and formatting by operating on yaml.Node AST.
type Document struct {
	root *yaml.Node
}

// Ensure Document implements document.Document interface.
var _ document.Document = (*Document)(nil)

// New creates a new empty YAML document.
func New() *Document {
	return &Document{
		root: &yaml.Node{
			Kind: yaml.DocumentNode,
			Content: []*yaml.Node{
				{Kind: yaml.MappingNode},
			},
		},
	}
}

// Parse parses YAML data into a Document.
//
// Empty/nil input is treated as an empty document.
func Parse(data []byte) (document.Document, error) {
	if len(data) == 0 {
		return New(), nil
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &Document{root: &root}, nil
}

// Format returns the document format.
func (d *Document) Format() document.DocumentFormat {
	return document.FormatYAML
}

// Marshal serializes the document to YAML bytes.
func (d *Document) Marshal() ([]byte, error) {
	data, err := yaml.Marshal(d.root)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal YAML: %w", err)
	}
	return data, nil
}

// Get retrieves the value at the specified JSON Pointer path.
func (d *Document) Get(path string) (any, bool) {
	keys, err := jsonptr.Parse(path)
	if err != nil {
		return nil, false
	}

	node := d.getRootMapping()
	if node == nil {
		return nil, false
	}

	// Empty path refers to the whole document
	if len(keys) == 0 {
		return nodeToValue(node), true
	}

	found := findNode(node, keys)
	if found == nil {
		return nil, false
	}

	return nodeToValue(found), true
}

// Set sets the value at the specified JSON Pointer path.
func (d *Document) Set(path string, value any) error {
	keys, err := jsonptr.Parse(path)
	if err != nil {
		return &document.InvalidPathError{Path: path, Reason: err.Error()}
	}

	// Empty path is not allowed for Set
	if len(keys) == 0 {
		return &document.InvalidPathError{Path: path, Reason: "cannot set root document"}
	}

	root := d.getRootMapping()
	if root == nil {
		return fmt.Errorf("document root is not a mapping")
	}

	return setNodeValue(root, keys, value)
}

// Delete removes the value at the specified JSON Pointer path.
func (d *Document) Delete(path string) error {
	keys, err := jsonptr.Parse(path)
	if err != nil {
		return &document.InvalidPathError{Path: path, Reason: err.Error()}
	}

	// Empty path is not allowed for Delete
	if len(keys) == 0 {
		return &document.InvalidPathError{Path: path, Reason: "cannot delete root document"}
	}

	root := d.getRootMapping()
	if root == nil {
		return fmt.Errorf("document root is not a mapping")
	}

	return deleteNode(root, keys)
}

// getRootMapping returns the root mapping node.
// YAML documents have a DocumentNode wrapper around the actual content.
func (d *Document) getRootMapping() *yaml.Node {
	if d.root.Kind == yaml.DocumentNode && len(d.root.Content) > 0 {
		return d.root.Content[0]
	}
	return d.root
}

// resolveAlias returns the actual node if the given node is an alias, otherwise returns the node itself.
func resolveAlias(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.AliasNode && node.Alias != nil {
		return node.Alias
	}
	return node
}

// findNode navigates to a node using a key path.
// Supports both object keys (strings) and array indices (numeric strings).
// Alias nodes are automatically resolved to their target.
func findNode(node *yaml.Node, keys []string) *yaml.Node {
	node = resolveAlias(node)
	if node == nil || len(keys) == 0 {
		return node
	}

	key := keys[0]
	remaining := keys[1:]

	switch node.Kind {
	case yaml.MappingNode:
		// Search for the key in the mapping
		for i := 0; i < len(node.Content)-1; i += 2 {
			keyNode := node.Content[i]
			valueNode := resolveAlias(node.Content[i+1])

			if keyNode.Value == key {
				if len(remaining) == 0 {
					return valueNode
				}
				return findNode(valueNode, remaining)
			}
		}
		return nil

	case yaml.SequenceNode:
		// Parse key as array index
		index, err := strconv.Atoi(key)
		if err != nil || index < 0 || index >= len(node.Content) {
			return nil
		}

		valueNode := resolveAlias(node.Content[index])
		if len(remaining) == 0 {
			return valueNode
		}
		return findNode(valueNode, remaining)

	default:
		return nil
	}
}

// setNodeValue sets a value at the specified key path.
// Creates intermediate nodes if they don't exist.
// Note: Setting values through alias nodes modifies the original anchor target.
func setNodeValue(node *yaml.Node, keys []string, value any) error {
	node = resolveAlias(node)
	if node == nil || len(keys) == 0 {
		return fmt.Errorf("invalid path")
	}

	key := keys[0]
	remaining := keys[1:]

	switch node.Kind {
	case yaml.MappingNode:
		// Search for existing key
		for i := 0; i < len(node.Content)-1; i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]

			if keyNode.Value == key {
				// Resolve alias to update the actual target
				valueNode = resolveAlias(valueNode)
				if len(remaining) == 0 {
					// Update the value in place
					updateNodeValue(valueNode, value)
					return nil
				}

				// Navigate deeper
				if valueNode.Kind != yaml.MappingNode && valueNode.Kind != yaml.SequenceNode {
					// Convert to mapping to allow deeper navigation
					valueNode.Kind = yaml.MappingNode
					valueNode.Tag = ""
					valueNode.Value = ""
					valueNode.Content = nil
				}
				return setNodeValue(valueNode, remaining, value)
			}
		}

		// Key doesn't exist - create it
		if len(remaining) == 0 {
			// Add leaf value
			node.Content = append(node.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: key},
				valueToNode(value),
			)
			return nil
		}

		// Check if the next key is a numeric index - create sequence instead of mapping
		var newNode *yaml.Node
		if _, err := strconv.Atoi(remaining[0]); err == nil {
			// Next key is numeric - create a sequence
			newNode = &yaml.Node{Kind: yaml.SequenceNode}
		} else {
			// Next key is not numeric - create a mapping
			newNode = &yaml.Node{Kind: yaml.MappingNode}
		}

		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: key},
			newNode,
		)
		return setNodeValue(newNode, remaining, value)

	case yaml.SequenceNode:
		// Parse key as array index
		index, err := strconv.Atoi(key)
		if err != nil {
			return &document.InvalidPathError{Path: key, Reason: "array index must be a number"}
		}

		if index < 0 || index > len(node.Content) {
			return &document.InvalidPathError{Path: key, Reason: fmt.Sprintf("array index %d out of range [0, %d]", index, len(node.Content))}
		}

		// Allow setting at index == len(Content) to append
		if index == len(node.Content) {
			if len(remaining) == 0 {
				node.Content = append(node.Content, valueToNode(value))
				return nil
			}

			// Check if the next key is a numeric index - create sequence instead of mapping
			var newNode *yaml.Node
			if _, err := strconv.Atoi(remaining[0]); err == nil {
				// Next key is numeric - create a sequence
				newNode = &yaml.Node{Kind: yaml.SequenceNode}
			} else {
				// Next key is not numeric - create a mapping
				newNode = &yaml.Node{Kind: yaml.MappingNode}
			}

			node.Content = append(node.Content, newNode)
			return setNodeValue(newNode, remaining, value)
		}

		// Update existing element
		valueNode := resolveAlias(node.Content[index])
		if len(remaining) == 0 {
			updateNodeValue(valueNode, value)
			return nil
		}

		if valueNode.Kind != yaml.MappingNode && valueNode.Kind != yaml.SequenceNode {
			valueNode.Kind = yaml.MappingNode
			valueNode.Tag = ""
			valueNode.Value = ""
			valueNode.Content = nil
		}
		return setNodeValue(valueNode, remaining, value)

	default:
		return &document.TypeMismatchError{
			Path:     key,
			Expected: "mapping or sequence",
			Actual:   nodeKindString(node.Kind),
		}
	}
}

// deleteNode removes a node at the specified key path.
// Note: Alias nodes are resolved when navigating deeper, but deleting an alias
// removes the alias itself, not the anchor target.
func deleteNode(node *yaml.Node, keys []string) error {
	node = resolveAlias(node)
	if node == nil || len(keys) == 0 {
		return fmt.Errorf("invalid path")
	}

	key := keys[0]
	remaining := keys[1:]

	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i < len(node.Content)-1; i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]

			if keyNode.Value == key {
				if len(remaining) == 0 {
					// Remove this key-value pair
					node.Content = append(node.Content[:i], node.Content[i+2:]...)
					return nil
				}
				// Resolve alias when navigating deeper
				return deleteNode(resolveAlias(valueNode), remaining)
			}
		}
		// Key not found - this is okay (idempotent)
		return nil

	case yaml.SequenceNode:
		index, err := strconv.Atoi(key)
		if err != nil || index < 0 || index >= len(node.Content) {
			// Index not valid - this is okay (idempotent)
			return nil
		}

		if len(remaining) == 0 {
			// Remove this element
			node.Content = append(node.Content[:index], node.Content[index+1:]...)
			return nil
		}

		// Resolve alias when navigating deeper
		return deleteNode(resolveAlias(node.Content[index]), remaining)

	default:
		return nil // Cannot delete from scalar - this is okay
	}
}

// updateNodeValue updates a node's value in place, preserving comments.
func updateNodeValue(node *yaml.Node, value any) {
	switch v := value.(type) {
	case string:
		node.Kind = yaml.ScalarNode
		node.Tag = "!!str"
		node.Value = v
		node.Content = nil

	case int:
		node.Kind = yaml.ScalarNode
		node.Tag = "!!int"
		node.Value = strconv.Itoa(v)
		node.Content = nil

	case int64:
		node.Kind = yaml.ScalarNode
		node.Tag = "!!int"
		node.Value = strconv.FormatInt(v, 10)
		node.Content = nil

	case float64:
		node.Kind = yaml.ScalarNode
		node.Tag = "!!float"
		node.Value = strconv.FormatFloat(v, 'f', -1, 64)
		node.Content = nil

	case bool:
		node.Kind = yaml.ScalarNode
		node.Tag = "!!bool"
		node.Value = strconv.FormatBool(v)
		node.Content = nil

	case nil:
		node.Kind = yaml.ScalarNode
		node.Tag = "!!null"
		node.Value = ""
		node.Content = nil

	case []any:
		node.Kind = yaml.SequenceNode
		node.Tag = ""
		node.Value = ""
		node.Content = make([]*yaml.Node, len(v))
		for i, elem := range v {
			node.Content[i] = valueToNode(elem)
		}

	case map[string]any:
		node.Kind = yaml.MappingNode
		node.Tag = ""
		node.Value = ""
		node.Content = make([]*yaml.Node, 0, len(v)*2)
		for k, val := range v {
			node.Content = append(node.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: k},
				valueToNode(val),
			)
		}

	default:
		// For other types, marshal and unmarshal through YAML
		data, _ := yaml.Marshal(v)
		var tempNode yaml.Node
		yaml.Unmarshal(data, &tempNode)
		if tempNode.Kind == yaml.DocumentNode && len(tempNode.Content) > 0 {
			*node = *tempNode.Content[0]
		}
	}
}

// valueToNode creates a new yaml.Node from a value.
func valueToNode(value any) *yaml.Node {
	node := &yaml.Node{}
	updateNodeValue(node, value)
	return node
}

// nodeToValue converts a yaml.Node to a Go value.
// Alias nodes are automatically resolved to their target value.
func nodeToValue(node *yaml.Node) any {
	node = resolveAlias(node)
	if node == nil {
		return nil
	}

	switch node.Kind {
	case yaml.ScalarNode:
		return parseScalarValue(node)

	case yaml.MappingNode:
		m := make(map[string]any)
		for i := 0; i < len(node.Content)-1; i += 2 {
			key := node.Content[i].Value
			m[key] = nodeToValue(node.Content[i+1])
		}
		return m

	case yaml.SequenceNode:
		s := make([]any, len(node.Content))
		for i, n := range node.Content {
			s[i] = nodeToValue(n)
		}
		return s

	default:
		return nil
	}
}

// parseScalarValue parses a scalar node value.
// It distinguishes between:
//   - null (explicit null or empty untagged value): returns nil
//   - empty string (!!str tag with empty value): returns ""
//   - zero values (0, false, etc.): returns the actual zero value
func parseScalarValue(node *yaml.Node) any {
	// Explicit null tag
	if node.Tag == "!!null" {
		return nil
	}

	// Explicit string tag - always return as string (including empty string)
	if node.Tag == "!!str" {
		return node.Value
	}

	// Empty value without tag is treated as null (YAML spec behavior)
	if node.Value == "" && node.Tag == "" {
		return nil
	}

	switch node.Tag {
	case "!!bool":
		b, _ := strconv.ParseBool(node.Value)
		return b

	case "!!int":
		i, _ := strconv.ParseInt(node.Value, 10, 64)
		return int(i)

	case "!!float":
		f, _ := strconv.ParseFloat(node.Value, 64)
		return f

	default:
		// Type tag not present - auto-detect
		if b, err := strconv.ParseBool(node.Value); err == nil {
			return b
		}
		if i, err := strconv.ParseInt(node.Value, 10, 64); err == nil {
			return int(i)
		}
		if f, err := strconv.ParseFloat(node.Value, 64); err == nil {
			return f
		}
		return node.Value
	}
}

// nodeKindString returns a human-readable string for a node kind.
func nodeKindString(kind yaml.Kind) string {
	switch kind {
	case yaml.DocumentNode:
		return "document"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.MappingNode:
		return "mapping"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	default:
		return "unknown"
	}
}

// MarshalTestData generates YAML bytes from the given data structure.
// YAML supports all common data structures, so this never returns
// UnsupportedStructureError.
func (d *Document) MarshalTestData(data map[string]any) ([]byte, error) {
	return yaml.Marshal(data)
}
