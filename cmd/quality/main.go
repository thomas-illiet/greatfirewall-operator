// Command quality checks GoDoc completeness and the exact project coverage threshold.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/greatfirewall/operator/internal/quality"
)

// run dispatches quality checks and writes deterministic reports to the supplied output.
func run(args []string, output io.Writer) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: quality docs <directory> | coverage <profile>")
	}
	if args[0] == "docs" {
		return quality.Documentation(args[1])
	}
	if args[0] != "coverage" {
		return fmt.Errorf("unknown quality command")
	}
	file, err := os.Open(args[1])
	if err != nil {
		return err
	}
	defer file.Close()
	return quality.Report(file, output)
}

// main returns a failing process status whenever a required quality gate fails.
func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
