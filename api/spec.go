// Package api holds the source HTTP contract.
package api

import _ "embed"

// openAPISpec is the contract generated models and runtime validation share.
//
//go:embed openapi.yaml
var openAPISpec string

// OpenAPISpec returns a copy of the source contract for runtime validation.
func OpenAPISpec() []byte {
	return []byte(openAPISpec)
}
