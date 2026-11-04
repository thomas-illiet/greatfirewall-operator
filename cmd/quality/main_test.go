package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestRun validates quality subcommands without terminating the test process.
func TestRun(t *testing.T) {
	dir := t.TempDir()
	if err := run([]string{"docs", dir}, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{}, {"unknown", dir}, {"coverage", "missing"}} {
		if run(args, io.Discard) == nil {
			t.Fatal("invalid invocation succeeded")
		}
	}
	path := filepath.Join(dir, "coverage.out")
	if err := os.WriteFile(path, []byte("mode: set\npkg/a.go:1.1,2.2 1 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"coverage", path}, io.Discard); err != nil {
		t.Fatal(err)
	}
}
