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
func (h *Handler) relayWeatherGrid(writer http.ResponseWriter, request *http.Request, response *http.Response) {
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
	//nolint:errcheck // A write that fails partway is the client disconnecting; nothing left to report it to.
	_, _ = io.Copy(writer, io.LimitReader(response.Body, maximumWeatherGridBytes))
}

func parseWeatherGridTime(raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, raw)

	return parsed, err == nil
}
