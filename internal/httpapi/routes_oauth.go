package httpapi

import (
	"net/http"
	"net/url"
	"slices"
)

// StartOAuth redirects the operator's browser to Wahoo for one target. A
// caller's own subject is always allowed and self-service creates the target
// on first use — the bare path with no target named defaults to it, since the
// browser is never told its own subject. Any other target is refused as not
// found unless the caller is admin — and even then only when it already
// exists, since only a rider's own "Connect" creates a target — so a
// non-admin cannot probe which other targets exist.
func (h *Handler) StartOAuth(writer http.ResponseWriter, request *http.Request) {
	// The state is bound to the caller's own subject: with more than one allowed
	// subject, a shared constant would let one operator complete another's
	// authorization.
	identity := identityOf(request.Context())
	targetID := request.PathValue("target")
	if targetID == "" {
		targetID = identity.Subject
	}
	if targetID == identity.Subject {
		if err := h.state.EnsureTargetOwner(request.Context(), identity.Subject); err != nil {
			h.unavailable(writer)

			return
		}
	} else {
		if !identity.Admin {
			h.notFound(writer)

			return
		}
		existing, err := h.targetIDs(request.Context())
		if err != nil {
			h.unavailable(writer)

			return
		}
		if !slices.Contains(existing, targetID) {
			h.notFound(writer)

			return
		}
	}

	location, err := h.oauth.Start(request.Context(), identity.Subject, targetID)
	if err != nil {
		h.error(writer, http.StatusBadRequest, "authorization_failed", "wahoo authorization could not be started")

		return
	}
	parsedLocation, parseErr := url.Parse(location)
	if parseErr != nil || parsedLocation.Scheme != "https" || parsedLocation.Host == "" {
		h.unavailable(writer)

		return
	}
	// Re-rendered from the parsed URL, so what the browser is handed is provably
	// the value the checks above accepted. Taint analysis cannot see those checks:
	// targetID is refused unless it names a configured slot, and the URL is refused
	// unless it parses as https with a host.
	//nolint:gosec // G710: redirect target is allowlisted and scheme-checked above.
	http.Redirect(writer, request, parsedLocation.String(), http.StatusFound)
}

// CompleteOAuth consumes the one-time OAuth state without echoing its query
// values. It returns the browser to the UI rather than the JSON status endpoint:
// the operator was sent to Wahoo by a link on that page. The 303 drops the
// authorization code and state from the browser URL.
func (h *Handler) CompleteOAuth(writer http.ResponseWriter, request *http.Request) {
	login := identityOf(request.Context()).Subject
	query := request.URL.Query()
	if err := h.oauth.Complete(request.Context(), login, query.Get("state"), query.Get("code")); err != nil {
		h.error(writer, http.StatusBadRequest, "authorization_failed", "wahoo authorization could not be completed")

		return
	}
	http.Redirect(writer, request, "/", http.StatusSeeOther)
}
