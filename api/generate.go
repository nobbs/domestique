//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/generator/golang"
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
	for _, output := range []string{
		"internal/httpapi/contract/openapi.gen.go",
		"internal/readiness/contract/openapi.gen.go",
	} {
		if err := os.WriteFile(output, generated.Source, 0o644); err != nil {
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
