//go:build ignore

package main

import (
	"fmt"
	"os"
	"regexp"
	"slices"

	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	"github.com/pb33f/libopenapi/generator/golang"
	"github.com/pb33f/libopenapi/orderedmap"
)

func main() {
	if err := generate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate() error {
	specification, err := os.ReadFile("api/openapi.yaml")
	if err != nil {
		return fmt.Errorf("reading OpenAPI contract: %w", err)
	}
	document, err := libopenapi.NewDocument(specification)
	if err != nil {
		return fmt.Errorf("reading OpenAPI contract: %w", err)
	}
	model, err := document.BuildV3Model()
	if err != nil {
		return fmt.Errorf("building OpenAPI model: %w", err)
	}
	generated, err := golang.NewGenerator(
		golang.WithPackageName("contract"),
		golang.WithGeneratedComment(true),
		golang.WithEnumConstants(true),
		golang.WithFormatMapping("date-time", "time.Time", "time"),
	).RenderSchemas(model.Model.Components.Schemas)
	if err != nil {
		return fmt.Errorf("rendering OpenAPI models: %w", err)
	}
	if err := checkDiagnostics(generated.Diagnostics); err != nil {
		return err
	}
	source, err := pointeriseNullableItems(
		generated.Source, nullableItemProperties(model.Model.Components.Schemas),
	)
	if err != nil {
		return err
	}
	for _, output := range []string{
		"internal/httpapi/contract/openapi.gen.go",
		"internal/readiness/contract/openapi.gen.go",
	} {
		if err := os.WriteFile(output, source, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", output, err)
		}
	}

	return nil
}

func checkDiagnostics(diagnostics []golang.Diagnostic) error {
	for _, diagnostic := range diagnostics {
		switch diagnostic.Code {
		case golang.DiagnosticAdditionalPropertiesFalse,
			golang.DiagnosticConstKeyword,
			golang.DiagnosticValidationKeyword:
			// Runtime contract validation, rather than generated Go structs,
			// enforces these schema constraints.
		default:
			return fmt.Errorf("OpenAPI model generation lost %s at %s: %s",
				diagnostic.Code, diagnostic.Path, diagnostic.Message)
		}
	}

	return nil
}

// nullableItemProperties names every property the document declares as an array
// whose items may be null.
func nullableItemProperties(schemas *orderedmap.Map[string, *base.SchemaProxy]) []string {
	var names []string
	seen := make(map[*base.Schema]struct{})

	var walk func(proxy *base.SchemaProxy)
	walk = func(proxy *base.SchemaProxy) {
		if proxy == nil {
			return
		}
		schema := proxy.Schema()
		if schema == nil {
			return
		}
		if _, visited := seen[schema]; visited {
			return
		}
		seen[schema] = struct{}{}

		if schema.Items != nil && schema.Items.IsA() {
			walk(schema.Items.A)
		}
		if schema.Properties == nil {
			return
		}
		for pair := schema.Properties.First(); pair != nil; pair = pair.Next() {
			if items := itemSchema(pair.Value()); items != nil && slices.Contains(items.Type, "null") {
				names = append(names, pair.Key())
			}
			walk(pair.Value())
		}
	}

	for pair := schemas.First(); pair != nil; pair = pair.Next() {
		walk(pair.Value())
	}
	slices.Sort(names)

	return slices.Compact(names)
}

func itemSchema(proxy *base.SchemaProxy) *base.Schema {
	schema := proxy.Schema()
	if schema == nil || schema.Items == nil || !schema.Items.IsA() {
		return nil
	}

	return schema.Items.A.Schema()
}

// pointeriseNullableItems repairs the one place the generator drops a document
// promise: it pointerises a nullable struct field but not a nullable array
// element, so `[number, "null"]` items render as []float64. A property that no
// longer needs the repair is an error, not a no-op — it means the generator has
// grown the fidelity this shim stands in for, and the shim should go.
func pointeriseNullableItems(source []byte, properties []string) ([]byte, error) {
	for _, property := range properties {
		field := regexp.MustCompile(
			`\[\](\w+)(\s+` + "`" + `json:"` + regexp.QuoteMeta(property) + `[,"])`,
		)
		if matches := field.FindAll(source, -1); len(matches) != 1 {
			return nil, fmt.Errorf(
				"pointerising nullable items of %q: matched %d generated fields, want 1",
				property, len(matches),
			)
		}
		source = field.ReplaceAll(source, []byte("[]*${1}${2}"))
	}

	return source, nil
}
