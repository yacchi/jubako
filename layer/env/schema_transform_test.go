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

func TestSchemaMapping_NestedTemplateTransform(t *testing.T) {
	// Scenario: Map containing Slice
	// Config -> Users (Map) -> Posts (Slice) -> Title
	// env: USERS_{key|lower}_POSTS_{index}_TITLE
	// path: /users/{key}/posts/{index}/title
	
	type Post struct {
		Title string `json:"title" jubako:"env:USERS_{key|lower}_POSTS_{index}_TITLE"`
	}
	
	type User struct {
		Posts []Post `json:"posts"`
	}
	
	type Config struct {
		Users map[string]User `json:"users"`
	}

	schema := BuildSchemaMapping[Config]()
	transform := schema.CreateTransformFunc()

	// Case 1: Matching input
	// key=ALICE -> lower=alice
	// index=0
	key := "USERS_ALICE_POSTS_0_TITLE"
	value := "My First Post"
	
	path, val := transform(key, value)
	
	expectedPath := "/users/alice/posts/0/title"
	
	if path != expectedPath {
		t.Errorf("Nested transform mismatch: want %q, got %q", expectedPath, path)
	}
	if val != "My First Post" {
		t.Errorf("Value mismatch: want %q, got %v", value, val)
	}

	// Case 2: Slice containing Map
	// Config -> Groups (Slice) -> Members (Map) -> Role
	// env: GROUPS_{index}_MEMBERS_{key|upper}_ROLE
	// path: /groups/{index}/members/{key}/role
	
	type Member struct {
		Role string `json:"role" jubako:"env:GROUPS_{index}_MEMBERS_{key|upper}_ROLE"`
	}
	
	type Group struct {
		Members map[string]Member `json:"members"`
	}
	
	type Config2 struct {
		Groups []Group `json:"groups"`
	}
	
	schema2 := BuildSchemaMapping[Config2]()
	transform2 := schema2.CreateTransformFunc()
	
	// index=1
	// key=bob -> upper=BOB
	key2 := "GROUPS_1_MEMBERS_bob_ROLE"
	value2 := "admin"
	
	path2, val2 := transform2(key2, value2)
	
	expectedPath2 := "/groups/1/members/BOB/role"
	
	if path2 != expectedPath2 {
		t.Errorf("Nested transform mismatch (Slice->Map): want %q, got %q", expectedPath2, path2)
	}
	if val2 != "admin" {
		t.Errorf("Value mismatch: want %q, got %v", value2, val2)
	}
}
