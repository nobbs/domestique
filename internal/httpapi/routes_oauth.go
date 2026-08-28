package httpapi

import (
	"net/http"
	"net/url"
	"slices"
)

// StartOAuth redirects the operator's browser to Wahoo for one configured slot.
func (h *Handler) StartOAuth(writer http.ResponseWriter, request *http.Request) {
	login := h.allowedEmail
	targetID := request.PathValue("target")
	if targetID == "" || !slices.Contains(h.targetIDs(), targetID) {
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
	// Re-rendered from the parsed URL rather than sent as the string that was
	// parsed, so what the browser is handed is provably the value the checks
	// above accepted.
	//
	// Taint analysis reaches the request through targetID, which reaches the
	// authorization URL through the OAuth service. It cannot see the two
	// checks that make the path safe: targetID is refused unless it is one of
	// the configured slots, and the URL the service returns is refused unless
	// it parses as https with a host. Neither is expressible to the analyser,
	// so the finding is suppressed here rather than answered by a third check.
	//nolint:gosec // G710: redirect target is allowlisted and scheme-checked above.
	http.Redirect(writer, request, parsedLocation.String(), http.StatusFound)
}

// CompleteOAuth consumes the one-time OAuth state without echoing its query values.
//
// It returns the browser to the UI rather than to the JSON status endpoint.
// What arrives here is an operator who was sent to Wahoo by a link on that
// page, and the page is where the target they just connected is described; the
// endpoint would answer the same question in a format nobody came to read. The
// redirect drops the authorization code and state from the browser URL either
// way, which is what the 303 is for.
func (h *Handler) CompleteOAuth(writer http.ResponseWriter, request *http.Request) {
	login := h.allowedEmail
	query := request.URL.Query()
	if err := h.oauth.Complete(request.Context(), login, query.Get("state"), query.Get("code")); err != nil {
		h.error(writer, http.StatusBadRequest, "authorization_failed", "wahoo authorization could not be completed")

		return
	}
	http.Redirect(writer, request, "/", http.StatusSeeOther)
}
