package httpapi

import (
	"log/slog"
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
	if !identity.Admin && targetID != identity.Subject {
		h.notFound(writer)

		return
	}
	if targetID == identity.Subject {
		if err := h.state.EnsureTargetOwner(request.Context(), identity.Subject); err != nil {
			h.unavailable(writer)

			return
		}
	}
	// Re-checked through the same identity-scoped list a status read uses,
	// even for a caller's own subject: a target already existing under a
	// misassigned owner_subject (only reachable by an operator editing state
	// by hand) must not let self-service quietly write to it anyway.
	existing, err := h.targetIDs(request.Context())
	if err != nil {
		h.unavailable(writer)

		return
	}
	if !slices.Contains(existing, targetID) {
		h.notFound(writer)

		return
	}

	location, err := h.oauth.Start(request.Context(), identity.Subject, targetID)
	if err != nil {
		slog.Warn("wahoo authorization refused", "reason", "start_failed")
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
//
// Where Wahoo refuses the request itself — a scope the Cloud application is not
// registered for, or a rider who declined — it issues no code at all and sends
// the browser here with `error=` instead, so that is checked before state and
// code are ever required.
func (h *Handler) CompleteOAuth(writer http.ResponseWriter, request *http.Request) {
	login := identityOf(request.Context()).Subject
	query := request.URL.Query()
	if errorCode := query.Get("error"); errorCode != "" {
		slog.Warn("wahoo authorization refused", "reason", refusalReason(errorCode))
		h.authorizationFailed(writer)

		return
	}
	state, code := query.Get("state"), query.Get("code")
	if state == "" {
		slog.Warn("wahoo authorization refused", "reason", "callback_state_missing")
		h.authorizationFailed(writer)

		return
	}
	if code == "" {
		slog.Warn("wahoo authorization refused", "reason", "callback_code_missing")
		h.authorizationFailed(writer)

		return
	}
	if err := h.oauth.Complete(request.Context(), login, state, code); err != nil {
		slog.Warn("wahoo authorization refused", "reason", "exchange_failed")
		h.authorizationFailed(writer)

		return
	}
	http.Redirect(writer, request, "/", http.StatusSeeOther)
}

// refusalReason names the two Wahoo refusals worth telling apart — a scope the
// application was never registered for, and a rider who said no — as fixed
// categories. The query value reaches this from a URL any signed-in caller can
// construct, so it is never logged as it arrived.
func refusalReason(errorCode string) string {
	switch errorCode {
	case "invalid_scope":
		return "scope_not_registered"
	case "access_denied":
		return "rider_declined"
	default:
		return "rejected_by_wahoo"
	}
}

func (h *Handler) authorizationFailed(writer http.ResponseWriter) {
	h.error(writer, http.StatusBadRequest, "authorization_failed", "wahoo authorization could not be completed")
}
