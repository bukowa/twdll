package extractdb_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryBoundary_NoSQLOrDBBypassInProtectedPackages(t *testing.T) {
	protectedDirs := []string{
		filepath.Join("..", "cmd"),
		filepath.Join("..", "silver"),
		filepath.Join("..", "bronze"),
	}
	forbiddenImports := map[string]bool{
		"database/sql":       true,
		"modernc.org/sqlite": true,
	}

	fset := token.NewFileSet()
	for _, dir := range protectedDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("ReadDir(%s): %v", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(dir, name)
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				t.Fatalf("ParseFile(%s): %v", path, err)
			}
			for _, imp := range file.Imports {
				impPath := strings.Trim(imp.Path.Value, `"`)
				if forbiddenImports[impPath] {
					t.Errorf("%s imports forbidden package %q (must stay encapsulated inside internal/extractdb)", filepath.ToSlash(path), impPath)
				}
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if ok && sel.Sel.Name == "DB" && len(call.Args) == 0 {
					pos := fset.Position(call.Pos())
					t.Errorf("%s:%d calls .DB() directly; use a typed extractdb repository method instead", filepath.ToSlash(path), pos.Line)
				}
				return true
			})
		}
	}
}
