package paths

import (
	"strings"
	"testing"
)

func TestToCamelCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"host", "Host"},
		{"server_port", "ServerPort"},
		{"http-read-timeout", "HttpReadTimeout"},
		{"some.nested.key", "SomeNestedKey"},
		{"already_Camel", "AlreadyCamel"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := toCamelCase(tt.input)
			if result != tt.expected {
				t.Errorf("toCamelCase(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGenerateConstName(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/server/port", "PathServerPort"},
		{"/server/host", "PathServerHost"},
		{"/database", "PathDatabase"},
		{"/server/http_timeout", "PathServerHttpTimeout"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := generateConstName(tt.path)
			if result != tt.expected {
				t.Errorf("generateConstName(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestGenerateFuncName(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/hosts/{key}/url", "PathHostsUrl"},
		{"/plugins/{index}/name", "PathPluginsName"},
		{"/servers/{key}/ports/{index}/address", "PathServersPortsAddress"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := generateFuncName(tt.path)
			if result != tt.expected {
				t.Errorf("generateFuncName(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestBuildFunctionBody(t *testing.T) {
	tests := []struct {
		path     string
		params   []ParamInfo
		contains []string
	}{
		{
			path:   "/hosts/{key}/url",
			params: []ParamInfo{{Name: "key", Type: "string"}},
			contains: []string{
				"jsonptr.Escape(key)",
				`"/hosts/"`,
				`"/url"`,
			},
		},
		{
			path:   "/plugins/{index}/name",
			params: []ParamInfo{{Name: "index", Type: "int"}},
			contains: []string{
				"strconv.Itoa(index)",
				`"/plugins/"`,
				`"/name"`,
			},
		},
		{
			path: "/servers/{key}/ports/{index}/address",
			params: []ParamInfo{
				{Name: "key", Type: "string"},
				{Name: "index", Type: "int"},
			},
			contains: []string{
				"jsonptr.Escape(key)",
				"strconv.Itoa(index)",
				`"/servers/"`,
				`"/ports/"`,
				`"/address"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := buildFunctionBody(tt.path, tt.params)
			for _, substr := range tt.contains {
				if !strings.Contains(result, substr) {
					t.Errorf("buildFunctionBody(%q) = %q, want to contain %q", tt.path, result, substr)
				}
			}
		})
	}
}

func TestParseJubakoTagPath(t *testing.T) {
	tests := []struct {
		tag        string
		wantPath   string
		wantAbsolute bool
	}{
		{`jubako:"/settings/global"`, "/settings/global", true},
		{`jubako:"hostname"`, "/hostname", false},
		{`jubako:"./relative/path"`, "/relative/path", false},
		{`jubako:"sensitive"`, "", false},
		{`jubako:"-"`, "", false},
		{`jubako:"/path,sensitive"`, "/path", true},
		{`json:"field"`, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			path, isAbsolute := parseJubakoTagPath(tt.tag)
			if path != tt.wantPath {
				t.Errorf("parseJubakoTagPath(%q) path = %q, want %q", tt.tag, path, tt.wantPath)
			}
			if isAbsolute != tt.wantAbsolute {
				t.Errorf("parseJubakoTagPath(%q) isAbsolute = %v, want %v", tt.tag, isAbsolute, tt.wantAbsolute)
			}
		})
	}
}

func TestGetFieldKey(t *testing.T) {
	tests := []struct {
		fieldName string
		tag       string
		tagName   string
		expected  string
	}{
		{"Host", `json:"host"`, "json", "host"},
		{"Port", `json:"port,omitempty"`, "json", "port"},
		{"Name", `yaml:"name"`, "yaml", "name"},
		{"Field", ``, "json", "Field"},
		{"Field", `json:"-"`, "json", "-"},
	}

	for _, tt := range tests {
		t.Run(tt.fieldName, func(t *testing.T) {
			result := getFieldKey(tt.fieldName, tt.tag, tt.tagName)
			if result != tt.expected {
				t.Errorf("getFieldKey(%q, %q, %q) = %q, want %q",
					tt.fieldName, tt.tag, tt.tagName, result, tt.expected)
			}
		})
	}
}

func TestGenerateCode(t *testing.T) {
	analysis := &AnalysisResult{
		Paths: []PathInfo{
			{
				JSONPointer: "/server/host",
				ConstName:   "PathServerHost",
				FieldName:   "Host",
			},
			{
				JSONPointer: "/server/port",
				ConstName:   "PathServerPort",
				FieldName:   "Port",
			},
			{
				JSONPointer:   "/hosts/{key}/url",
				FuncName:      "PathHostsUrl",
				FieldName:     "URL",
				DynamicParams: []ParamInfo{{Name: "key", Type: "string"}},
				Comment:       "Path pattern: /hosts/{key}/url",
			},
		},
	}

	config := GeneratorConfig{
		PackageName: "config",
		TypeName:    "AppConfig",
		SourceFile:  "config.go",
		TagName:     "json",
	}

	code, err := generateCode(analysis, config)
	if err != nil {
		t.Fatalf("generateCode() error = %v", err)
	}

	codeStr := string(code)

	// Check header
	if !strings.Contains(codeStr, "Code generated by jubako generate paths") {
		t.Error("generated code should contain header comment")
	}

	// Check package
	if !strings.Contains(codeStr, "package config") {
		t.Error("generated code should contain package declaration")
	}

	// Check constants
	if !strings.Contains(codeStr, "PathServerHost") {
		t.Error("generated code should contain PathServerHost constant")
	}
	if !strings.Contains(codeStr, `"/server/host"`) {
		t.Error("generated code should contain /server/host path")
	}

	// Check function
	if !strings.Contains(codeStr, "func PathHostsUrl(key string) string") {
		t.Error("generated code should contain PathHostsUrl function")
	}
	if !strings.Contains(codeStr, "jsonptr.Escape(key)") {
		t.Error("generated code should use jsonptr.Escape")
	}

	// Check imports
	if !strings.Contains(codeStr, `"github.com/yacchi/jubako/jsonptr"`) {
		t.Error("generated code should import jsonptr")
	}
}
