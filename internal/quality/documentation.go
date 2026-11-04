// Package quality enforces documentation, maintainability and exact test coverage gates.
package quality

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// Documentation checks every declaration, including tests, and production source size limits.
func Documentation(root string) error {
	var issues []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		found, err := inspectSource(path)
		issues = append(issues, found...)
		return err
	})
	if err != nil {
		return err
	}
	if len(issues) == 0 {
		return nil
	}

	sort.Strings(issues)
	return fmt.Errorf("source quality checks failed:\n%s", strings.Join(issues, "\n"))
}

// inspectSource enforces GoDoc for named declarations and keeps production functions small.
func inspectSource(path string) ([]string, error) {
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var issues []string
	production := !strings.HasSuffix(path, "_test.go")
	if production && set.Position(file.End()).Line > 250 {
		issues = append(issues, path+": production file exceeds 250 lines")
	}

	ast.Inspect(file, func(node ast.Node) bool {
		switch declaration := node.(type) {
		case *ast.FuncDecl:
			position := set.Position(declaration.Pos())
			if !documented(declaration.Doc, declaration.Name.Name) {
				issues = append(issues, fmt.Sprintf("%s: missing GoDoc for %s", position, declaration.Name.Name))
			}
			if production && set.Position(declaration.End()).Line-position.Line+1 > 60 {
				issues = append(issues, fmt.Sprintf("%s: %s exceeds 60 lines", position, declaration.Name.Name))
			}
		case *ast.InterfaceType:
			issues = append(issues, interfaceIssues(set, declaration)...)
		}
		return true
	})
	return issues, nil
}

// interfaceIssues verifies comments on methods without requiring comments on embedded interfaces.
func interfaceIssues(set *token.FileSet, declaration *ast.InterfaceType) []string {
	var issues []string
	for _, field := range declaration.Methods.List {
		if _, method := field.Type.(*ast.FuncType); !method {
			continue
		}
		for _, name := range field.Names {
			if !documented(field.Doc, name.Name) {
				issues = append(issues, fmt.Sprintf("%s: missing GoDoc for %s", set.Position(field.Pos()), name.Name))
			}
		}
	}
	return issues
}

// documented requires a substantive GoDoc comment beginning with the declaration name.
func documented(comment *ast.CommentGroup, name string) bool {
	if comment == nil {
		return false
	}
	text := strings.TrimSpace(comment.Text())
	return strings.HasPrefix(text, name+" ") && len(strings.Fields(text)) >= 3
}
