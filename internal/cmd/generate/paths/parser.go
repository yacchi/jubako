package paths

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"

	"golang.org/x/tools/go/packages"
)

// ParsedPackage holds the parsed package information.
type ParsedPackage struct {
	Name string
	Path string
}

// parseSourceFile parses the source file and returns the package info and struct type.
func parseSourceFile(sourceFile string, typeName string) (*ParsedPackage, *types.Struct, error) {
	absPath, err := filepath.Abs(sourceFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	dir := filepath.Dir(absPath)

	// Load the package
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo,
		Dir:  dir,
	}

	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load package: %w", err)
	}

	if len(pkgs) == 0 {
		return nil, nil, fmt.Errorf("no packages found")
	}

	pkg := pkgs[0]
	if len(pkg.Errors) > 0 {
		return nil, nil, fmt.Errorf("package errors: %v", pkg.Errors)
	}

	// Find the struct type
	obj := pkg.Types.Scope().Lookup(typeName)
	if obj == nil {
		return nil, nil, fmt.Errorf("type %s not found in package %s", typeName, pkg.Name)
	}

	typeObj, ok := obj.Type().Underlying().(*types.Struct)
	if !ok {
		return nil, nil, fmt.Errorf("%s is not a struct type", typeName)
	}

	return &ParsedPackage{
		Name: pkg.Name,
		Path: pkg.PkgPath,
	}, typeObj, nil
}

// parseSourceFileAST parses the source file using AST only (for simpler cases).
// This is an alternative implementation that doesn't require full type checking.
func parseSourceFileAST(sourceFile string, typeName string) (*ParsedPackage, *ast.TypeSpec, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, sourceFile, nil, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse file: %w", err)
	}

	// Find the type declaration
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			if typeSpec.Name.Name == typeName {
				return &ParsedPackage{
					Name: file.Name.Name,
				}, typeSpec, nil
			}
		}
	}

	return nil, nil, fmt.Errorf("type %s not found in %s", typeName, sourceFile)
}
