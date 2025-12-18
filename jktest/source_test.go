package jktest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yacchi/jubako/jktest"
	"github.com/yacchi/jubako/source"
	"github.com/yacchi/jubako/source/bytes"
	"github.com/yacchi/jubako/source/fs"
)

func TestBytesSource_Compliance(t *testing.T) {
	factory := func(data []byte) source.Source {
		return bytes.New(data)
	}
	jktest.NewSourceTester(t, factory).TestAll()
}

func TestFsSource_Compliance(t *testing.T) {
	factory := func(data []byte) source.Source {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		return fs.New(path)
	}

	// NotExistFactory creates a Source pointing to a non-existent file
	notExistFactory := func() source.Source {
		dir := t.TempDir()
		return fs.New(filepath.Join(dir, "nonexistent.json"))
	}

	jktest.NewSourceTester(t, factory,
		jktest.WithNotExistFactory(notExistFactory),
	).TestAll()
}
