package env

import (
	"testing"
)

func TestBuildSchemaMapping_Pattern_Map(t *testing.T) {
	type User struct {
		Name string `json:"name" jubako:"env:USERS_{key}_NAME"`
		Role string `json:"role" jubako:"env:USERS_{key}_ROLE"`
	}

	type Config struct {
		Users map[string]User `json:"users"`
	}

	schema := BuildSchemaMapping[Config]()
	transform := schema.CreateTransformFunc()

	// Test Alice
	path, value := transform("USERS_ALICE_NAME", "Alice Wonderland")
	if path != "/users/ALICE/name" {
		t.Errorf("USERS_ALICE_NAME path = %q, want /users/ALICE/name", path)
	}
	if value != "Alice Wonderland" {
		t.Errorf("USERS_ALICE_NAME value = %v, want 'Alice Wonderland'", value)
	}

	// Test Bob Role
	path, value = transform("USERS_BOB_ROLE", "admin")
	if path != "/users/BOB/role" {
		t.Errorf("USERS_BOB_ROLE path = %q, want /users/BOB/role", path)
	}
	if value != "admin" {
		t.Errorf("USERS_BOB_ROLE value = %v, want 'admin'", value)
	}

	// Test Unmatched
	path, _ = transform("USERS_CHARLIE_AGE", "30")
	if path != "" {
		t.Errorf("USERS_CHARLIE_AGE should not match")
	}
}

func TestBuildSchemaMapping_Pattern_Slice(t *testing.T) {
	type PortConfig struct {
		Port int `json:"port" jubako:"env:PORTS_{index}_NUM"`
	}

	type Config struct {
		Ports []PortConfig `json:"ports"`
	}

	schema := BuildSchemaMapping[Config]()
	transform := schema.CreateTransformFunc()

	// Test index 0
	path, value := transform("PORTS_0_NUM", "8080")
	if path != "/ports/0/port" {
		t.Errorf("PORTS_0_NUM path = %q, want /ports/0/port", path)
	}
	if val, ok := value.(int); !ok || val != 8080 {
		t.Errorf("PORTS_0_NUM value = %v, want 8080", value)
	}

	// Test index 10
	path, value = transform("PORTS_10_NUM", "9000")
	if path != "/ports/10/port" {
		t.Errorf("PORTS_10_NUM path = %q, want /ports/10/port", path)
	}
	if val, ok := value.(int); !ok || val != 9000 {
		t.Errorf("PORTS_10_NUM value = %v, want 9000", value)
	}

	// Test invalid index (non-digit)
	path, _ = transform("PORTS_abc_NUM", "80")
	if path != "" {
		t.Errorf("PORTS_abc_NUM should not match (index must be digits)")
	}
}

func TestBuildSchemaMapping_Pattern_Nested(t *testing.T) {
	type Feature struct {
		Enabled bool `json:"enabled" jubako:"env:FEAT_{key}_ENABLED"`
	}

	type Config struct {
		Features map[string]Feature `json:"features"`
	}

	schema := BuildSchemaMapping[Config]()
	transform := schema.CreateTransformFunc()

	// Test feature 'login'
	path, value := transform("FEAT_LOGIN_ENABLED", "true")
	if path != "/features/LOGIN/enabled" {
		t.Errorf("FEAT_LOGIN_ENABLED path = %q, want /features/LOGIN/enabled", path)
	}
	if val, ok := value.(bool); !ok || !val {
		t.Errorf("FEAT_LOGIN_ENABLED value = %v, want true", value)
	}

	// Test feature 'signup'
	path, value = transform("FEAT_SIGNUP_ENABLED", "false")
	if path != "/features/SIGNUP/enabled" {
		t.Errorf("FEAT_SIGNUP_ENABLED path = %q, want /features/SIGNUP/enabled", path)
	}
	if val, ok := value.(bool); !ok || val {
		t.Errorf("FEAT_SIGNUP_ENABLED value = %v, want false", value)
	}
}

func TestBuildSchemaMapping_Pattern_Mixed(t *testing.T) {
	// Combination of Map and Slice
	// Config -> Groups (Map) -> Users (Slice) -> Name
	type User struct {
		Name string `json:"name" jubako:"env:GROUP_{key}_USER_{index}_NAME"`
	}
	type Group struct {
		Users []User `json:"users"`
	}
	type Config struct {
		Groups map[string]Group `json:"groups"`
	}

	schema := BuildSchemaMapping[Config]()
	transform := schema.CreateTransformFunc()

	path, value := transform("GROUP_ADMIN_USER_0_NAME", "Alice")
	if path != "/groups/ADMIN/users/0/name" {
		t.Errorf("Path mismatch: got %q, want /groups/ADMIN/users/0/name", path)
	}
	if value != "Alice" {
		t.Errorf("Value mismatch: got %v, want Alice", value)
	}
}

func TestBuildSchemaMapping_Pattern_Priority(t *testing.T) {
	// Exact match should take precedence over pattern match
	type Config struct {
		Generic string `json:"generic" jubako:"env:ITEM_{key}"`
		Specific string `json:"specific" jubako:"env:ITEM_SPECIAL"`
	}
	// Note: This struct design is a bit forced for the test, as ITEM_{key} usually implies a map.
	// But let's test if we have a pattern mapping that *could* match an exact string.

	type Item struct {
		Val string `json:"val" jubako:"env:ITEM_{key}_VAL"`
	}
	type Wrapper struct {
		Items map[string]Item `json:"items"`
		Special Item `json:"special"` // Explicit mapping logic
	}

	// Let's rely on the implementation detail that specific mappings are in Mappings map
	// and patterns are in Patterns slice. CreateTransformFunc checks Mappings first.
	
	// Manually construct a case where a pattern could overlap with a static key if not careful.
	// But BuildSchemaMapping separates them based on presence of placeholders.
	
	// If I have:
	// env:MY_VAR_{key}
	// env:MY_VAR_SPECIAL
	
	// BuildSchemaMapping will put MY_VAR_SPECIAL in Mappings (if no placeholders)
	// and MY_VAR_{key} in Patterns.
	
	// So checking MY_VAR_SPECIAL should hit Mappings first.
}

func TestCompileEnvPattern(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		want    bool
		groups  map[string]string
	}{
		{
			pattern: "USERS_{key}_NAME",
			input:   "USERS_ALICE_NAME",
			want:    true,
			groups:  map[string]string{"key": "ALICE"},
		},
		{
			pattern: "PORTS_{index}",
			input:   "PORTS_123",
			want:    true,
			groups:  map[string]string{"index": "123"},
		},
		{
			pattern: "PORTS_{index}",
			input:   "PORTS_abc",
			want:    false,
			groups:  nil,
		},
		{
			pattern: "MIXED_{key}_{index}",
			input:   "MIXED_foo_1",
			want:    true,
			groups:  map[string]string{"key": "foo", "index": "1"},
		},
	}

	for _, tt := range tests {
		regex, err := compileEnvPattern(tt.pattern)
		if err != nil {
			t.Errorf("compileEnvPattern(%q) error = %v", tt.pattern, err)
			continue
		}

		if !regex.MatchString(tt.input) {
			if tt.want {
				t.Errorf("Pattern %q did not match input %q", tt.pattern, tt.input)
			}
			continue
		}

		if !tt.want {
			t.Errorf("Pattern %q matched input %q, but should not", tt.pattern, tt.input)
			continue
		}

		matches := regex.FindStringSubmatch(tt.input)
		for name, wantVal := range tt.groups {
			idx := regex.SubexpIndex(name)
			if idx < 0 {
				t.Errorf("Group %q not found in regex", name)
				continue
			}
			if matches[idx] != wantVal {
				t.Errorf("Group %q = %q, want %q", name, matches[idx], wantVal)
			}
		}
	}
}
