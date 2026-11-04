package quality

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Counts holds exact covered and total instruction counts, without rounding.
type Counts struct {
	Covered int64
	Total   int64
}

// Pass reports whether at least ninety percent of instructions are covered.
func (c Counts) Pass() bool { return c.Total > 0 && c.Covered*100 >= c.Total*90 }

// Coverage parses a Go coverage profile, merging duplicate blocks without inflating coverage.
func Coverage(reader io.Reader) (Counts, map[string]Counts, error) {
	scanner := bufio.NewScanner(reader)
	if !scanner.Scan() || (scanner.Text() != "mode: atomic" && scanner.Text() != "mode: count" && scanner.Text() != "mode: set") {
		return Counts{}, nil, errors.New("invalid coverage header")
	}
	blocks := map[string]Counts{}
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			return Counts{}, nil, errors.New("invalid coverage row")
		}
		statements, err := strconv.ParseInt(fields[1], 10, 32)
		if err != nil || statements < 0 {
			return Counts{}, nil, errors.New("invalid statement count")
		}
		hits, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || hits < 0 {
			return Counts{}, nil, errors.New("invalid execution count")
		}
		if !strings.Contains(fields[0], ".go:") {
			return Counts{}, nil, errors.New("invalid source location")
		}
		previous, exists := blocks[fields[0]]
		if exists && previous.Total != statements {
			return Counts{}, nil, errors.New("conflicting coverage blocks")
		}
		covered := previous.Covered
		if hits > 0 {
			covered = statements
		}
		blocks[fields[0]] = Counts{Covered: covered, Total: statements}
	}
	if err := scanner.Err(); err != nil {
		return Counts{}, nil, err
	}
	total := Counts{}
	packages := map[string]Counts{}
	for location, count := range blocks {
		file := strings.Split(location, ".go:")[0] + ".go"
		pkg := filepath.Dir(file)
		current := packages[pkg]
		current.Covered += count.Covered
		current.Total += count.Total
		packages[pkg] = current
		total.Covered += count.Covered
		total.Total += count.Total
	}
	return total, packages, nil
}

// Report writes global and per-package evidence and fails below the exact ninety percent gate.
func Report(reader io.Reader, writer io.Writer) error {
	total, packages, err := Coverage(reader)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(packages))
	for name := range packages {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		count := packages[name]
		fmt.Fprintf(writer, "%s: %d/%d statements\n", name, count.Covered, count.Total)
	}
	fmt.Fprintf(writer, "TOTAL: %d/%d statements (%.4f%%)\n", total.Covered, total.Total, 100*float64(total.Covered)/float64(total.Total))
	if !total.Pass() {
		return errors.New("coverage is below 90%")
	}
	return nil
}
