package json

import (
	"testing"

	"github.com/yacchi/jubako/jktest"
)

func TestJSONDocument_Compliance(t *testing.T) {
	jktest.NewDocumentTester(t, NewParser()).TestAll()
}

func TestParser_Parse(t *testing.T) {
	p := NewParser()

	doc, err := p.Parse([]byte(`{"a":1,"b":{"c":true},"items":["x","y"]}`))
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
