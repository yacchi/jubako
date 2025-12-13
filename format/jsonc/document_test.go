package jsonc

import (
	"strings"
	"testing"

	"github.com/yacchi/jubako/jktest"
)

func TestJSONCDocument_Compliance(t *testing.T) {
	jktest.NewDocumentTester(t, NewParser()).TestAll()
}

func TestParser_Parse(t *testing.T) {
	p := NewParser()

	doc, err := p.Parse([]byte("{\n  // comment\n  \"a\": 1,\n  \"b\": {\"c\": true},\n  \"items\": [\"x\", \"y\",],\n}\n"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if got, ok := doc.Get("/a"); !ok || got != float64(1) {
		t.Fatalf("Get(/a) = %v (ok=%v), want 1", got, ok)
	}
	if got, ok := doc.Get("/b/c"); !ok || got != true {
		t.Fatalf("Get(/b/c) = %v (ok=%v), want true", got, ok)
	}
	if got, ok := doc.Get("/items/0"); !ok || got != "x" {
		t.Fatalf("Get(/items/0) = %v (ok=%v), want \"x\"", got, ok)
	}
}

func TestParse_Empty(t *testing.T) {
	doc, err := Parse([]byte(" \n\t"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if root, ok := doc.Get(""); !ok || root == nil {
		t.Fatalf("Get(\"\") = %v (ok=%v), want non-nil root map", root, ok)
	}
}

func TestDocument_CommentPreservation(t *testing.T) {
	input := []byte("{\n  // heading\n  \"server\": {\n    \"host\": \"localhost\" // inline\n  }\n}\n")

	doc, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if err := doc.Set("/server/port", 9000); err != nil {
		t.Fatalf("Set(/server/port) error = %v", err)
	}

	out, err := doc.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	s := string(out)

	if !strings.Contains(s, "// heading") {
		t.Error("Marshal() did not preserve heading comment")
	}
	if !strings.Contains(s, "inline") {
		t.Error("Marshal() did not preserve inline comment")
	}
	if !strings.Contains(s, "\"port\"") || !strings.Contains(s, "9000") {
		t.Error("Marshal() did not include updated value")
	}
}
