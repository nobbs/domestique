package sync

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The runbook is written against the categories this package emits, and those
// are the only words an operator ever sees for a run that did not succeed. A
// category added here without an entry there leaves the one document that
// explains it silently incomplete, which is exactly the failure a runbook exists
// to prevent.
//
// The categories are read out of the declaration rather than listed again here,
// because a list this test maintains by hand would pass for exactly the change
// it exists to catch.
func TestTheRunbookExplainsEveryFailureCategory(t *testing.T) {
	runbook := readRunbook(t)

	categories := declaredFailureCategories(t)
	require.NotEmpty(t, categories, "no failure categories were found in result.go")
	for _, category := range categories {
		assert.Contains(t, runbook, "`"+category+"`",
			"docs/runbook.md does not explain the %q failure category", category)
	}
}

// declaredFailureCategories reads every non-empty FailureCategory constant from
// this package's own source. FailureNone is the absence of a reason rather than
// one of them, so it is left out.
func declaredFailureCategories(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "result.go", nil, 0)
	require.NoError(t, err)

	var categories []string
	for _, declaration := range file.Decls {
		general, isGeneral := declaration.(*ast.GenDecl)
		if !isGeneral || general.Tok != token.CONST {
			continue
		}
		for _, spec := range general.Specs {
			value, named := failureCategoryValue(spec)
			if named && value != "" {
				categories = append(categories, value)
			}
		}
	}

	return categories
}

// failureCategoryValue reports the string a constant spec assigns, and whether
// that constant is a FailureCategory at all.
func failureCategoryValue(spec ast.Spec) (string, bool) {
	valueSpec, isValue := spec.(*ast.ValueSpec)
	if !isValue || len(valueSpec.Values) != 1 {
		return "", false
	}
	name, isName := valueSpec.Type.(*ast.Ident)
	if !isName || name.Name != "FailureCategory" {
		return "", false
	}
	literal, isLiteral := valueSpec.Values[0].(*ast.BasicLit)
	if !isLiteral || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	if err != nil {
		return "", false
	}

	return value, true
}

func readRunbook(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	contents, err := os.ReadFile(filepath.Join(root, "docs", "runbook.md")) //nolint:gosec // a repository document, named by this test
	require.NoError(t, err)

	return string(contents)
}
