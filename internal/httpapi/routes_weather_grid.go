package httpapi

import (
	"io"
	"net/http"
	"time"
)

// maximumWeatherGridBytes bounds one relayed body. latest.json is a few
// kilobytes; a range read is chunk-sized, a few hundred kilobytes at most.
// This is headroom over both without being unbounded, guarding against a
// malformed Range header asking for far more than the reader ever does.
const maximumWeatherGridBytes = 8 << 20

// GetWeatherGridLatest relays the model's own capture manifest, so the
// browser's reader learns the current run and valid times without reaching
// Open-Meteo directly.
func (h *Handler) GetWeatherGridLatest(writer http.ResponseWriter, request *http.Request) {
	response, err := h.weatherGrid.Latest(request.Context())
	if err != nil {
		h.error(writer, http.StatusBadGateway, "provider_unavailable", "the weather provider could not be reached")

		return
	}
	//nolint:errcheck // A response body that will not close cannot change the result.
	defer func() { _ = response.Body.Close() }()
	h.relayWeatherGrid(writer, request, response)
}

// GetWeatherGridObject relays one .om file's bytes, or answers a HEAD, for
// the run and hour the query names. The browser's reader chooses its own
// byte ranges and sends them as a Range header, forwarded here unchanged.
func (h *Handler) GetWeatherGridObject(writer http.ResponseWriter, request *http.Request) {
	referenceTime, referenceOk := parseWeatherGridTime(request.URL.Query().Get("referenceTime"))
	validTime, validOk := parseWeatherGridTime(request.URL.Query().Get("validTime"))
	if !referenceOk || !validOk {
		h.error(writer, http.StatusBadRequest, "invalid_request", "referenceTime and validTime must be RFC3339 timestamps")

		return
	}

	response, err := h.weatherGrid.Object(
		request.Context(), referenceTime, validTime, request.Method, request.Header.Get("Range"),
	)
	if err != nil {
		h.error(writer, http.StatusBadGateway, "provider_unavailable", "the weather provider could not be reached")

		return
	}
	//nolint:errcheck // A response body that will not close cannot change the result.
	defer func() { _ = response.Body.Close() }()
	h.relayWeatherGrid(writer, request, response)
}

// relayWeatherGrid writes the upstream's status and named headers, then its
// body unless the request was HEAD — matching how GET /healthz already
// answers HEAD without a body.
//
// A HEAD's Content-Length names bytes nobody asks for next, so it is relayed
// as-is; a GET's is a promise this relay then has to keep. Refusing before
// any byte is written, rather than truncating mid-response, is what keeps a
// too-large upstream from handing a client a Content-Length that the body
// underneath it does not actually reach — a malformed response a client can
// hang or error on rather than a clean failure.
func (h *Handler) relayWeatherGrid(writer http.ResponseWriter, request *http.Request, response *http.Response) {
	// An unsatisfiable Range is the caller's own request, mirrored back as
	// theirs — not a sign the provider is unreachable.
	if response.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		h.error(writer, http.StatusBadRequest, "invalid_request", "the requested byte range is not satisfiable")

		return
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		h.error(writer, http.StatusBadGateway, "provider_unavailable", "the weather provider could not be reached")

		return
	}
	if request.Method != http.MethodHead &&
		response.ContentLength >= 0 && response.ContentLength > maximumWeatherGridBytes {
		h.error(writer, http.StatusBadGateway, "provider_unavailable",
			"the weather provider returned a response larger than this relay allows")

		return
	}

	// The response headers a relay carries over from the upstream, verbatim.
	// Nothing else of the upstream response is trusted: no cookie, no
	// redirect, nothing naming the provider by name.
	header := writer.Header()
	for _, name := range []string{
		"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified",
	} {
		if value := response.Header.Get(name); value != "" {
			header.Set(name, value)
		}
	}
	writer.WriteHeader(response.StatusCode)
	if request.Method == http.MethodHead {
		return
	}
	// Bounded by the length just checked when the upstream reported one, so a
	// truthful Content-Length is exactly how many bytes follow it; the flat
	// cap only still matters when the upstream reports none at all.
	limit := int64(maximumWeatherGridBytes)
	if response.ContentLength >= 0 {
		limit = response.ContentLength
	}
	//nolint:errcheck // A write that fails partway is the client disconnecting; nothing left to report it to.
	_, _ = io.Copy(writer, io.LimitReader(response.Body, limit))
}

func parseWeatherGridTime(raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, false
	}
	// Nano, not RFC3339: the browser sends Date.toISOString()'s fractional
	// seconds, matching routes_activities.go's own query-timestamp parsing.
	parsed, err := time.Parse(time.RFC3339Nano, raw)

	return parsed, err == nil
}
