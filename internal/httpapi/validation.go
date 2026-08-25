package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	middleware "github.com/oapi-codegen/nethttp-middleware"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
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

// servedSpec is the contract this listener enforces: the embedded document
// minus the operations the readiness listener owns.
//
// The two have to be filtered separately. The generated router beside this is
// produced with exclude-tags, so it never registers /readyz, but embedded-spec
// carries the document whole — and a validator built from the unfiltered one
// would accept a readiness request on the served socket that the router then
// could not answer. Filtering by the tag rather than by the path keeps the
// document the source of that decision.
func servedSpec() (*openapi3.T, error) {
	spec, err := openapi.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("reading the embedded contract: %w", err)
	}
	for path, item := range spec.Paths.Map() {
		for method, operation := range item.Operations() {
			if slices.Contains(operation.Tags, readinessTag) {
				item.SetOperation(method, nil)
			}
		}
		if len(item.Operations()) == 0 {
			spec.Paths.Delete(path)
		}
	}

	return spec, nil
}

// useContractValidation builds the middleware that holds every request to the
// document: the parameter bounds, the request bodies, and the provenance
// requirement that used to be a hand-written table of paths.
//
// The middleware constructor panics rather than returning an error when it
// cannot build a router from the document, so the panic is recovered into one
// here: a startup failure belongs to main, which alone decides the exit code.
func (h *Handler) useContractValidation() (err error) {
	spec, err := servedSpec()
	if err != nil {
		return err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("building the contract validator: %v", recovered)
		}
	}()

	h.validate = middleware.OapiRequestValidatorWithOptions(spec, &middleware.Options{
		// The document declares one relative server, which would otherwise turn
		// into Host validation this service does not want: it is reached under
		// several names, and the identity gate is what decides who may call it.
		DoNotValidateServers: true,
		Options:              openapi3filter.Options{AuthenticationFunc: h.authenticateScheme},
		ErrorHandlerWithOpts: h.contractValidationError,
	})

	return nil
}

// authenticateScheme answers the security requirements the document declares.
//
// Only the provenance scheme is answered here. The Access assertion is proven
// by gated() before this middleware runs, and it must stay there: this
// validator resolves the route before it validates security, so an unknown path
// would answer 404 to a caller that has proven nothing, which is the surface
// enumeration the identity gate exists to prevent. Verifying the assertion a
// second time here would cost a second signature check per request and change
// nothing.
func (h *Handler) authenticateScheme(_ context.Context, input *openapi3filter.AuthenticationInput) error {
	if input.SecuritySchemeName != browserOriginScheme {
		return nil
	}
	// A browser attaches Origin to every request whose method is not GET or
	// HEAD, including a same-origin one, so the UI's own requests always carry
	// it. A missing header is therefore not "same-origin, header omitted" — it
	// is a caller that is not this UI. So is "null", which is what a sandboxed
	// or redirected context sends. Both fail this comparison.
	if input.RequestValidationInput.Request.Header.Get("Origin") != h.browserOrigin {
		return errForeignOrigin
	}

	return nil
}

// contractValidationError turns a refusal into this service's own error shape,
// so a request refused by the document reads the same as one refused by a
// handler.
func (h *Handler) contractValidationError(
	_ context.Context, err error, writer http.ResponseWriter, _ *http.Request, opts middleware.ErrorHandlerOpts,
) {
	security, refused := errors.AsType[*openapi3filter.SecurityRequirementsError](err)
	if refused && security != nil {
		h.error(writer, http.StatusForbidden, "forbidden", "request origin is not permitted")

		return
	}
	// A path the document does not name, and a method it does not name on a
	// path it does, are both "no such resource" here. ServeMux's unmatched-path
	// fallback answers the first that way already, and the contract declares no
	// 405 on any operation.
	if opts.StatusCode == http.StatusNotFound || opts.StatusCode == http.StatusMethodNotAllowed {
		h.notFound(writer)

		return
	}
	h.error(writer, http.StatusBadRequest, "invalid_request", "the request does not match this operation")
}
