package document

import (
	"testing"
)

// TestDocumentFormatString tests that DocumentFormat constants have correct values.
func TestDocumentFormatString(t *testing.T) {
	tests := []struct {
		format   DocumentFormat
		expected string
	}{
		{FormatYAML, "yaml"},
		{FormatTOML, "toml"},
		{FormatJSONC, "jsonc"},
		{FormatJSON, "json"},
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			if string(tt.format) != tt.expected {
				t.Errorf("DocumentFormat = %q, want %q", tt.format, tt.expected)
			}
		})
	}
}
