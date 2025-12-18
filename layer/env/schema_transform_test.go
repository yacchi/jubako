package env

import (
	"testing"
)

func TestSchemaMapping_TemplateTransform(t *testing.T) {
	// 1. Verify default behavior (case preservation)
	type Config struct {
		Backlog map[string]struct {
			ClientID string `json:"client_id" jubako:"env:BACKLOG_CLIENT_ID_{key}"`
		} `json:"backlog"`
	}

	schema := BuildSchemaMapping[Config]()
	transform := schema.CreateTransformFunc()

	key := "BACKLOG_CLIENT_ID_JP"
	value := "my-client-id"

	path, _ := transform(key, value)
	
	// Default behavior should preserve case (escaped)
	expectedDefaultPath := "/backlog/JP/client_id"
	if path != expectedDefaultPath {
		t.Errorf("Default behavior mismatch: want %q, got %q", expectedDefaultPath, path)
	}

	// 2. Verify transform with template filter
	type ConfigNew struct {
		Backlog map[string]struct {
			ClientID string `json:"client_id" jubako:"env:BACKLOG_CLIENT_ID_{key|lower}"`
		} `json:"backlog"`
	}

	schemaNew := BuildSchemaMapping[ConfigNew]()
	transformNew := schemaNew.CreateTransformFunc()
	
	pathNew, _ := transformNew(key, value)
	
	// Transformed behavior should be lowercase
	expectedTransformedPath := "/backlog/jp/client_id"
	if pathNew != expectedTransformedPath {
		t.Errorf("Transform behavior mismatch: want %q, got %q", expectedTransformedPath, pathNew)
	}

	// 3. Verify multiple filters (if we supported chaining in the regex parsing, 
	// but currently our implementation supports one filter after pipe.
	// However, one can theoretically use whatever is supported by text/template if we passed it raw,
	// but our regex parsing explicitly captures `(?:\|([^}]+))?`.
	// So `key|lower|upper` would be captured as filter=`lower|upper`.
	// `{{.key | lower|upper | escape}}` should work!
	
	type ConfigChain struct {
		Backlog map[string]struct {
			ClientID string `json:"client_id" jubako:"env:BACKLOG_CLIENT_ID_{key|lower|upper}"`
		} `json:"backlog"`
	}
	
	schemaChain := BuildSchemaMapping[ConfigChain]()
	transformChain := schemaChain.CreateTransformFunc()
	
	pathChain, _ := transformChain(key, value)
	// JP -> lower -> jp -> upper -> JP
	if pathChain != expectedDefaultPath {
		t.Errorf("Chain behavior mismatch: want %q, got %q", expectedDefaultPath, pathChain)
	}
}