package mapdoc

import "testing"

func TestNew_DefaultDataIsEmptyMap(t *testing.T) {
	d := New("test")
	if d.Data() == nil {
		t.Fatalf("Data() = nil, want non-nil map")
	}
	if got := len(d.Data()); got != 0 {
		t.Fatalf("len(Data()) = %d, want 0", got)
	}

	if err := d.Set("/a", 1); err != nil {
		t.Fatalf("Set(/a) error = %v", err)
	}
	if got, ok := d.Get("/a"); !ok || got != 1 {
		t.Fatalf("Get(/a) = %v (ok=%v), want 1", got, ok)
	}
}

func TestWithData_UsesProvidedMap(t *testing.T) {
	data := map[string]any{"a": 1}
	d := New("test", WithData(data))
	if got, ok := d.Get("/a"); !ok || got != 1 {
		t.Fatalf("Get(/a) = %v (ok=%v), want 1", got, ok)
	}

	d.Data()["b"] = 2
	if got, ok := data["b"]; !ok || got != 2 {
		t.Fatalf("provided data[b] = %v (ok=%v), want 2", got, ok)
	}
}

func TestWithData_NilBecomesEmptyMap(t *testing.T) {
	d := New("test", WithData(nil))
	if d.Data() == nil {
		t.Fatalf("Data() = nil, want non-nil map")
	}
}
