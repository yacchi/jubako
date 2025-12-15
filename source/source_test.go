package source

import (
	"errors"
	"testing"
)

func TestErrors(t *testing.T) {
	if ErrSaveNotSupported == nil || ErrSourceModified == nil {
		t.Fatal("expected non-nil sentinel errors")
	}
	if !errors.Is(ErrSaveNotSupported, ErrSaveNotSupported) {
		t.Fatal("errors.Is(ErrSaveNotSupported, ErrSaveNotSupported) = false, want true")
	}
	if errors.Is(ErrSaveNotSupported, ErrSourceModified) {
		t.Fatal("errors.Is(ErrSaveNotSupported, ErrSourceModified) = true, want false")
	}
}
