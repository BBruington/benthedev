package core

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// dataPackagePrefixes are the packages only core may hand around. depguard stops other packages from
// importing them, but Go infers types, so an exported core function returning *store.Queries or a
// pgx pool would let any caller reach the database without an import. This test closes that hole
// (CLAUDE.md "Boundaries").
var dataPackagePrefixes = []string{
	"github.com/BBruington/colony-planner/internal/store",
	"github.com/jackc/pgx",
	"database/sql",
}

// allowedDataReferences are the deliberate exceptions, named as they appear in core's API.
var allowedDataReferences = map[string]string{
	"Options.Pool":        "cmd/colony hands core its database pool at startup",
	"MemberStatus":        "the member_status enum, re-exported so callers never import store",
	"MemberStatusPending": "a member_status value",
	"MemberStatusActive":  "a member_status value",
	"MemberStatusRemoved": "a member_status value",
}

func TestCoreAPIExposesNoDataTypes(t *testing.T) {
	fileSet := token.NewFileSet()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var violations []string
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fileSet, path, source, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		dataImportNames := dataImportNamesOf(file)
		if len(dataImportNames) == 0 {
			continue
		}
		report := func(name string, node ast.Node) {
			if _, allowed := allowedDataReferences[name]; allowed {
				return
			}
			if mentionsDataPackage(node, dataImportNames) {
				violations = append(violations, fileSet.Position(node.Pos()).String()+": "+name)
			}
		}
		for _, declaration := range file.Decls {
			switch typed := declaration.(type) {
			case *ast.FuncDecl:
				if typed.Name.IsExported() && exportedReceiver(typed) {
					report(qualifiedFunctionName(typed), typed.Type)
				}
			case *ast.GenDecl:
				for _, specification := range typed.Specs {
					checkSpecification(specification, report)
				}
			}
		}
	}
	for _, violation := range violations {
		t.Errorf("core's exported API mentions a data type (store, pgx or database/sql): %s. "+
			"Return a core type instead, or add a deliberate exception to allowedDataReferences.", violation)
	}
}

func checkSpecification(specification ast.Spec, report func(string, ast.Node)) {
	switch typed := specification.(type) {
	case *ast.TypeSpec:
		if !typed.Name.IsExported() {
			return
		}
		structType, isStruct := typed.Type.(*ast.StructType)
		if !isStruct || typed.Assign.IsValid() {
			report(typed.Name.Name, typed.Type)
			return
		}
		for _, field := range structType.Fields.List {
			for _, fieldName := range field.Names {
				if fieldName.IsExported() {
					report(typed.Name.Name+"."+fieldName.Name, field.Type)
				}
			}
			if len(field.Names) == 0 {
				report(typed.Name.Name+" (embedded)", field.Type)
			}
		}
	case *ast.ValueSpec:
		for index, valueName := range typed.Names {
			if !valueName.IsExported() {
				continue
			}
			if typed.Type != nil {
				report(valueName.Name, typed.Type)
			}
			if index < len(typed.Values) {
				report(valueName.Name, typed.Values[index])
			}
		}
	}
}

// dataImportNamesOf maps each data package's local name in this file (its alias, or the last path
// element) so selector expressions like store.Queries can be recognized.
func dataImportNamesOf(file *ast.File) map[string]bool {
	names := map[string]bool{}
	for _, importSpecification := range file.Imports {
		importPath, err := strconv.Unquote(importSpecification.Path.Value)
		if err != nil || !isDataPackage(importPath) {
			continue
		}
		localName := importPath[strings.LastIndex(importPath, "/")+1:]
		if majorVersionSuffix.MatchString(localName) {
			// github.com/jackc/pgx/v5 is imported as pgx.
			trimmed := strings.TrimSuffix(importPath, "/"+localName)
			localName = trimmed[strings.LastIndex(trimmed, "/")+1:]
		}
		if importSpecification.Name != nil {
			localName = importSpecification.Name.Name
		}
		names[localName] = true
	}
	return names
}

var majorVersionSuffix = regexp.MustCompile(`^v[0-9]+$`)

func isDataPackage(importPath string) bool {
	for _, prefix := range dataPackagePrefixes {
		if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
			return true
		}
	}
	return false
}

func mentionsDataPackage(node ast.Node, dataImportNames map[string]bool) bool {
	found := false
	ast.Inspect(node, func(child ast.Node) bool {
		selector, isSelector := child.(*ast.SelectorExpr)
		if !isSelector {
			return !found
		}
		if packageName, isIdentifier := selector.X.(*ast.Ident); isIdentifier && dataImportNames[packageName.Name] {
			found = true
		}
		return !found
	})
	return found
}

// exportedReceiver is true for plain functions and for methods on exported types.
func exportedReceiver(function *ast.FuncDecl) bool {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return true
	}
	receiverType := function.Recv.List[0].Type
	if pointer, isPointer := receiverType.(*ast.StarExpr); isPointer {
		receiverType = pointer.X
	}
	identifier, isIdentifier := receiverType.(*ast.Ident)
	return isIdentifier && identifier.IsExported()
}

func qualifiedFunctionName(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return function.Name.Name
	}
	receiverType := function.Recv.List[0].Type
	if pointer, isPointer := receiverType.(*ast.StarExpr); isPointer {
		receiverType = pointer.X
	}
	if identifier, isIdentifier := receiverType.(*ast.Ident); isIdentifier {
		return identifier.Name + "." + function.Name.Name
	}
	return function.Name.Name
}
