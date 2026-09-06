package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"github.com/pb33f/libopenapi-validator/cache"
	"github.com/pb33f/libopenapi-validator/config"
	validationerrors "github.com/pb33f/libopenapi-validator/errors"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"

	api "github.com/nobbs/domestique/api"
)

// browserOriginScheme is the contract's name for the provenance requirement.
// Every operation that starts a run or writes state declares it, and that
// declaration is now the only place the list of such operations exists.
const browserOriginScheme = "browserOrigin"

// readinessTag marks the operations the loopback probe listener owns.
const readinessTag = "readiness"

// errForeignOrigin is what an operation's provenance requirement fails with.
// The reason travels no further than the status: telling a caller which check
// refused it describes the check it has to defeat.
var errForeignOrigin = errors.New("request origin is not permitted")

// servedSpec is the contract this listener enforces: the source document minus
// the operations the readiness listener owns. The generated router is produced
// with exclude-tags and never registers /readyz, so a validator built from the
// unfiltered document would accept a request the router could not answer.
func servedSpec() (*v3.Document, error) {
	document, err := libopenapi.NewDocument(api.OpenAPISpec())
	if err != nil {
		return nil, fmt.Errorf("reading the source contract: %w", err)
	}
	model, err := document.BuildV3Model()
	if err != nil {
		return nil, fmt.Errorf("building the source contract: %w", err)
	}
	for path, item := range model.Model.Paths.PathItems.FromOldest() {
		removeReadinessOperation(&item.Get)
		removeReadinessOperation(&item.Put)
		removeReadinessOperation(&item.Post)
		removeReadinessOperation(&item.Delete)
		removeReadinessOperation(&item.Options)
		removeReadinessOperation(&item.Head)
		removeReadinessOperation(&item.Patch)
		removeReadinessOperation(&item.Trace)
		removeReadinessOperation(&item.Query)
		if item.GetOperations().Len() == 0 {
			model.Model.Paths.PathItems.Delete(path)
		}
	}

	return &model.Model, nil
}

func removeReadinessOperation(operation **v3.Operation) {
	if *operation != nil && slices.Contains((*operation).Tags, readinessTag) {
		*operation = nil
	}
}

// useContractValidation builds the middleware that holds API and OAuth
// requests to the document: path and header bounds, request bodies, and the
// provenance requirement that used to be a hand-written table of paths. The
// handlers validate query parameters.
func (h *Handler) useContractValidation(schemaCache cache.SchemaCache) error {
	spec, err := servedSpec()
	if err != nil {
		return err
	}
	if schemaCache == nil {
		schemaCache = cache.NewDefaultCache()
	}
	contractValidator := validator.NewValidatorFromV3Model(
		spec,
		config.WithSchemaCache(schemaCache),
		config.WithAuthenticationFunc(h.authenticateScheme),
		// Weather points are repeated query parameters whose values contain commas,
		// which libopenapi-validator treats as array delimiters even when exploded.
		// The handlers already validate every query parameter this service has.
		// ponytail: restore when libopenapi-validator supports comma-bearing
		// exploded values.
		config.WithoutRequestQueryParameterValidation(),
	)

	h.validate = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if valid, validationErrors := contractValidator.ValidateHttpRequestSync(request); !valid {
				h.contractValidationError(writer, validationErrors)

				return
			}
			next.ServeHTTP(writer, request)
		})
	}

	return nil
}

// authenticateScheme answers the security requirements the document declares.
// Only the provenance scheme is answered here: the Access assertion is proven by
// gated() before this middleware runs, and must stay there, because this
// validator resolves the route before it validates security.
func (h *Handler) authenticateScheme(_ context.Context, input *config.AuthenticationInput) error {
	if input.SecuritySchemeName != browserOriginScheme {
		return nil
	}
	// A browser attaches Origin to every request whose method is not GET or HEAD,
	// including a same-origin one. A missing header is a caller that is not this
	// UI, and so is "null", which a sandboxed or redirected context sends.
	if input.Request.Header.Get("Origin") != h.browserOrigin {
		return errForeignOrigin
	}

	return nil
}

// contractValidationError turns a refusal into this service's own error shape,
// so a request refused by the document reads the same as one refused by a
// handler.
func (h *Handler) contractValidationError(
	writer http.ResponseWriter, validationErrors []*validationerrors.ValidationError,
) {
	if slices.ContainsFunc(validationErrors, func(validationErr *validationerrors.ValidationError) bool {
		return validationErr.Reason == errForeignOrigin.Error()
	}) {
		h.error(writer, http.StatusForbidden, "forbidden", "request origin is not permitted")

		return
	}
	// A path the document does not name, and a method it does not name on a
	// path it does, are both "no such resource" here. ServeMux's unmatched-path
	// fallback answers the first that way already, and the contract declares no
	// 405 on any operation.
	if slices.ContainsFunc(validationErrors, func(validationErr *validationerrors.ValidationError) bool {
		return validationErr.IsPathMissingError() || validationErr.IsOperationMissingError()
	}) {
		h.notFound(writer)

		return
	}
	h.error(writer, http.StatusBadRequest, "invalid_request", "the request does not match this operation")
}
