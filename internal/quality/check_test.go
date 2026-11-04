package quality

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDocumentation checks private functions, methods and interface methods, including tests.
func TestDocumentation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example_test.go")
	valid := `package sample
// private documents an unexported function.
func private() {}
type Contract interface {
 // Apply applies a change.
 Apply()
}
type Object struct{}
// update documents a private method.
func (Object) update() {}
`
	if err := os.WriteFile(path, []byte(valid), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Documentation(dir); err != nil {
		t.Fatal(err)
	}
	invalid := strings.ReplaceAll(valid, "// private documents an unexported function.", "")
	invalid = strings.ReplaceAll(invalid, "// Apply applies a change.", "")
	if err := os.WriteFile(path, []byte(invalid), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Documentation(dir); err == nil || !strings.Contains(err.Error(), "Apply") {
		t.Fatal("missing private or interface documentation accepted")
	}
	if err := os.WriteFile(path, []byte("broken go source"), 0600); err != nil {
		t.Fatal(err)
	}
	if Documentation(dir) == nil || Documentation(filepath.Join(dir, "absent")) == nil {
		t.Fatal("parse or filesystem failure swallowed")
	}
}

// TestCoverage counts exact instructions, merges duplicate blocks and rejects 89.95 percent.
func TestCoverage(t *testing.T) {
	text := "mode: atomic\npkg/a.go:1.1,2.2 90 1\npkg/a.go:3.1,4.2 10 0\n"
	total, packages, err := Coverage(strings.NewReader(text))
	if err != nil || total.Covered != 90 || total.Total != 100 || !total.Pass() || len(packages) != 1 {
		t.Fatalf("invalid counts: %+v %v", total, err)
	}
	duplicate := text + "pkg/a.go:3.1,4.2 10 1\n"
	total, _, err = Coverage(strings.NewReader(duplicate))
	if err != nil || total.Covered != 100 || total.Total != 100 {
		t.Fatal("duplicate blocks inflated denominator")
	}
	if (Counts{Covered: 1799, Total: 2000}).Pass() || (Counts{}).Pass() {
		t.Fatal("rounded-up or empty coverage accepted")
	}
	var output bytes.Buffer
	if err := Report(strings.NewReader(text), &output); err != nil || !strings.Contains(output.String(), "90.0000%") {
		t.Fatal("coverage report missing")
	}
	if Report(strings.NewReader("mode: set\npkg/a.go:1.1,2.2 1 0\n"), io.Discard) == nil {
		t.Fatal("zero coverage passed")
	}
}

// brokenReader simulates an interrupted coverage artifact read.
type brokenReader struct{}

// Read reports a deterministic I/O failure after the buffered header is consumed.
func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

// TestCoverageErrors rejects malformed reports rather than silently reducing the denominator.
func TestCoverageErrors(t *testing.T) {
	for _, text := range []string{
		"",
		"mode: unknown\n",
		"mode: set\ninvalid\n",
		"mode: set\npkg/a.go:1.1,2.2 -1 1\n",
		"mode: set\npkg/a.go:1.1,2.2 1 -1\n",
		"mode: set\nunknown 1 1\n",
		"mode: set\npkg/a.go:1.1,2.2 1 1\npkg/a.go:1.1,2.2 2 1\n",
	} {
		if Report(strings.NewReader(text), io.Discard) == nil {
			t.Fatalf("invalid profile accepted: %q", text)
		}
	}
	reader := io.MultiReader(strings.NewReader("mode: count\n"), brokenReader{})
	if _, _, err := Coverage(reader); err == nil {
		t.Fatal("read failure swallowed")
	}
}

// TestMaintainabilityGates rejects oversized production functions and files, without scanning vendors.
func TestMaintainabilityGates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.go")
	longFunction := "package example\n// long documents this function.\nfunc long() {\n"
	longFunction += strings.Repeat("_ = 1\n", 61) + "}\n"
	if err := os.WriteFile(path, []byte(longFunction), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Documentation(dir); err == nil || !strings.Contains(err.Error(), "60 lines") {
		t.Fatal("oversized function accepted")
	}

	longFile := "package example\n" + strings.Repeat("// separation\n", 251)
	longFile += "// small documents this function.\nfunc small() {}\n"
	if err := os.WriteFile(path, []byte(longFile), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Documentation(dir); err == nil || !strings.Contains(err.Error(), "250 lines") {
		t.Fatal("oversized file accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	vendor := filepath.Join(dir, "vendor")
	if err := os.Mkdir(vendor, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vendor, "foreign.go"), []byte("not our code"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Documentation(dir); err != nil {
		t.Fatal(err)
	}
}
