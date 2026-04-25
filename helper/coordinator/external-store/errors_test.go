package externalstore

import (
	"errors"
	"testing"
)

func TestNotExistError_Is(t *testing.T) {
	underlying := errors.New("backend missing")
	err := NewNotExistError("secret/key", underlying)

	if !errors.Is(err, ErrNotExist) {
		t.Fatal("errors.Is(err, ErrNotExist) = false, want true")
	}
	if !errors.Is(err, underlying) {
		t.Fatal("errors.Is(err, underlying) = false, want true")
	}
}
