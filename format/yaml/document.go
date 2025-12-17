// Package yaml provides a YAML implementation of the document.Document interface.
// It preserves comments and formatting by operating on yaml.Node AST during Apply.
package yaml

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/yacchi/jubako/document"
	"github.com/yacchi/jubako/jsonptr"
	"gopkg.in/yaml.v3"
)

// Document is a Document implementation for YAML format.
// It is stateless - parsing and serialization happen on demand.
type Document struct{}

// Ensure Document implements document.Document interface.
var _ document.Document = (*Document)(nil)

// New returns a YAML Document.
//
// Example:
//
//	src := fs.New("~/.config/app.yaml")
//	layer.New("user", src, yaml.New())
func New() *Document {
	return &Document{}
}

// Format returns the document format.
func (d *Document) Format() document.DocumentFormat {
	return document.FormatYAML
}

// Get parses data bytes and returns content as map[string]any.
// Returns empty map if data is nil or empty.
func (d *Document) Get(data []byte) (map[string]any, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}

	var result map[string]any
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if result == nil {
		return map[string]any{}, nil
	}

	return result, nil
}

// Apply applies changeset to data bytes and returns new bytes.
// If changeset is provided: parses data, applies changeset operations
// to preserve comments, then marshals the result.
// If changeset is empty: parses and re-marshals data directly.
func (d *Document) Apply(data []byte, changeset document.JSONPatchSet) ([]byte, error) {
	// If no changeset, parse and re-marshal (for format consistency)
	if changeset.IsEmpty() {
		var m map[string]any
		if len(bytes.TrimSpace(data)) > 0 {
			if err := yaml.Unmarshal(data, &m); err != nil {
				return nil, fmt.Errorf("failed to parse YAML: %w", err)
			}
		}
		if m == nil {
			m = map[string]any{}
		}
		return d.marshal(m)
	}

	// Parse existing data to preserve comments
	var root *yaml.Node
	if len(bytes.TrimSpace(data)) > 0 {
		var node yaml.Node
		if err := yaml.Unmarshal(data, &node); err != nil {
			// If parse fails, create new document
			root = &yaml.Node{
				Kind: yaml.DocumentNode,
				Content: []*yaml.Node{
					{Kind: yaml.MappingNode},
				},
			}
		} else {
			root = &node
		}
	} else {
		// No existing data, create new document structure
		root = &yaml.Node{
			Kind: yaml.DocumentNode,
			Content: []*yaml.Node{
				{Kind: yaml.MappingNode},
			},
		}
	}

	// Get the root mapping node
	rootMapping := getRootMapping(root)

	// Apply each patch operation
	for _, patch := range changeset {
		keys, err := jsonptr.Parse(patch.Path)
		if err != nil {
			continue // Skip invalid paths
		}

		switch patch.Op {
		case document.PatchOpAdd, document.PatchOpReplace:
			if len(keys) > 0 {
				setNodeValue(rootMapping, keys, patch.Value)
			}
		case document.PatchOpRemove:
			if len(keys) > 0 {
				deleteNode(rootMapping, keys)
			}
		}
	}

	return d.marshal(root)
}

// MarshalTestData generates YAML bytes from the given data structure.
// YAML supports all common data structures, so this never returns
// UnsupportedStructureError.
func (d *Document) MarshalTestData(data map[string]any) ([]byte, error) {
	return d.marshal(data)
}

// marshal encodes data to YAML with standard indentation (2 spaces).
func (d *Document) marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// getRootMapping returns the root mapping node.
// YAML documents have a DocumentNode wrapper around the actual content.
func getRootMapping(root *yaml.Node) *yaml.Node {
	if root == nil {
		return nil
	}
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		return root.Content[0]
	}
	if root.Kind == yaml.DocumentNode && len(root.Content) == 0 {
		m := &yaml.Node{Kind: yaml.MappingNode}
		root.Content = []*yaml.Node{m}
		return m
	}
	return root
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

// setNodeValue sets a value at the specified key path.
// Creates intermediate nodes if they don't exist.
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
