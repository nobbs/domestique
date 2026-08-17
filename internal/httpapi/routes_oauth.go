package httpapi

import (
	"net/http"
	"net/url"
	"slices"
)

// start redirects the operator's browser to Wahoo for one configured slot.
func (h *Handler) start(writer http.ResponseWriter, request *http.Request, login string) {
	targetID := request.PathValue("target")
	if targetID == "" || !slices.Contains(h.targetIDs, targetID) {
		h.notFound(writer)

		return
	}

	location, err := h.oauth.Start(request.Context(), login, targetID)
	if err != nil {
		h.error(writer, http.StatusBadRequest, "authorization_failed", "wahoo authorization could not be started")

		return
	}
	parsedLocation, parseErr := url.Parse(location)
	if parseErr != nil || parsedLocation.Scheme != "https" || parsedLocation.Host == "" {
		h.unavailable(writer)

		return
	}
	//nolint:gosec // The OAuth service returned a validated HTTPS Wahoo authorization URL.
	http.Redirect(writer, request, location, http.StatusFound)
}

// callback consumes the one-time OAuth state without echoing its query values.
func (h *Handler) callback(writer http.ResponseWriter, request *http.Request, login string) {
	query := request.URL.Query()
	if err := h.oauth.Complete(request.Context(), login, query.Get("state"), query.Get("code")); err != nil {
		h.error(writer, http.StatusBadRequest, "authorization_failed", "wahoo authorization could not be completed")

		return
	}
	http.Redirect(writer, request, "/v1/status", http.StatusSeeOther)
}
