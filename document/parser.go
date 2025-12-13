package document

// Parser parses raw bytes into a Document.
// Each format (YAML, TOML, JSONC, etc.) implements this interface.
type Parser interface {
	// Parse parses the raw bytes and returns a Document.
	Parse(data []byte) (Document, error)

	// Format returns the document format this parser handles.
	Format() DocumentFormat

	// CanMarshal returns true if documents created by this parser
	// can be marshaled back to bytes while preserving comments and formatting.
	// Parsers that cannot preserve comments (e.g., standard JSON) should return false.
	CanMarshal() bool

	// MarshalTestData generates bytes that, when parsed, produce a document
	// containing the given data structure. This is intended for testing.
	//
	// This method delegates to the Document's MarshalTestData after creating
	// an empty document via Parse.
	//
	// Returns UnsupportedStructureError if the data contains structures
	// that cannot be represented in this document format.
	MarshalTestData(data map[string]any) ([]byte, error)
}
