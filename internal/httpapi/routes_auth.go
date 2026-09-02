package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/nobbs/domestique/internal/session"
)

// signInFailed is what every failure but a refused subject says. One message
// for all of them: which step failed describes the check a caller has to defeat.
const signInFailed = "Sign-in could not be completed."

// GetLoginPage serves the static document that starts a sign-in. It writes
// nothing: a state is minted by the POST the button sends, so a crawler or a
// prefetch cannot fill the login table.
func (h *Handler) GetLoginPage(writer http.ResponseWriter, _ *http.Request) {
	h.page(writer, http.StatusOK, "login.html", nil)
}

// sameOrigin is the provenance check for the two routes outside the OpenAPI
// document: StartLogin's full-page form post and Logout's fetch call. Origin
// matching browserOrigin exactly is always sufficient, as it always was. The
// one addition is narrow: Safari answers a top-level POST navigation from a
// page served with Referrer-Policy: no-referrer — this login page,
// deliberately — with Origin: null, and Sec-Fetch-Site: same-origin is the
// browser's own corroboration for exactly that value. Sec-Fetch-Site is never
// trusted for any other Origin, because it reflects same-origin relative to
// whatever host the request actually reached, not this service's configured
// one — a request that reached this process under a different hostname could
// compute same-origin against that hostname and still name it in Origin.
func (h *Handler) sameOrigin(request *http.Request) bool {
	origin := request.Header.Get("Origin")
	if origin == h.browserOrigin {
		return true
	}

	return origin == "null" && request.Header.Get("Sec-Fetch-Site") == "same-origin"
}

// StartLogin mints a sign-in and redirects the browser to the provider. The
// Origin check is the one the contract's browserOrigin scheme makes; these
// routes sit outside the document, so it is made here.
func (h *Handler) StartLogin(writer http.ResponseWriter, request *http.Request) {
	if !h.sameOrigin(request) {
		h.page(writer, http.StatusForbidden, "denied.html", pageValues{Message: signInFailed})

		return
	}

	login, err := h.sessions.Begin(request.Context())
	if err != nil {
		slog.Warn("sign-in refused", "reason", "login_not_started")
		h.page(writer, http.StatusBadRequest, "denied.html", pageValues{Message: signInFailed})

		return
	}
	location, parseErr := url.Parse(login.AuthorizationURL)
	if parseErr != nil || location.Scheme != "https" || location.Host == "" {
		slog.Warn("sign-in refused", "reason", "authorization_url_unusable")
		h.page(writer, http.StatusBadRequest, "denied.html", pageValues{Message: signInFailed})

		return
	}
	h.setLoginCookie(writer, login.State)
	// Re-rendered from the parsed URL, so what the browser is handed is provably
	// the value the check above accepted; taint analysis cannot see that check.
	http.Redirect(writer, request, location.String(), http.StatusSeeOther)
}

// CompleteLogin finishes a sign-in and issues the session cookie. Where the
// Action itself refuses a subject, Auth0 never issues a code at all — the
// browser lands here with `error=access_denied` instead, so that is checked
// before state and code are ever required. error_description is tenant
// text and is never rendered; only the fixed category matters to a reader.
func (h *Handler) CompleteLogin(writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	if errorCode := query.Get("error"); errorCode != "" {
		message := signInFailed
		reason := "authorization_denied"
		if errorCode == "access_denied" {
			message = "This account is not allowed to sign in."
			reason = "subject_not_allowed"
		}
		slog.Warn("sign-in refused", "reason", reason)
		h.clearCookie(writer, loginCookie)
		h.page(writer, http.StatusForbidden, "denied.html", pageValues{Message: message})

		return
	}
	state, code := query.Get("state"), query.Get("code")
	cookie, cookieErr := request.Cookie(loginCookie)
	if cookieErr != nil || state == "" || code == "" || cookie.Value == "" {
		slog.Warn("sign-in refused", "reason", "login_state_missing")
		// Cleared like the branches below: a pending state that got this far is
		// spent, and leaving it set has the browser resend it until it expires.
		h.clearCookie(writer, loginCookie)
		h.page(writer, http.StatusBadRequest, "denied.html", pageValues{Message: signInFailed})

		return
	}

	completion, err := h.sessions.Complete(request.Context(), state, cookie.Value, code)
	var notAllowed *session.NotAllowedError
	switch {
	case errors.As(err, &notAllowed):
		// The one place the refused subject is shown: a reader who cannot get in
		// needs to know which account was refused. The log line carries the
		// category only.
		slog.Warn("sign-in refused", "reason", "subject_not_allowed")
		h.clearCookie(writer, loginCookie)
		h.page(writer, http.StatusForbidden, "denied.html", pageValues{
			Message: "This account is not allowed to sign in.",
			Subject: notAllowed.Subject,
		})

		return
	case err != nil:
		slog.Warn("sign-in refused", "reason", "exchange_failed")
		h.clearCookie(writer, loginCookie)
		h.page(writer, http.StatusBadRequest, "denied.html", pageValues{Message: signInFailed})

		return
	}

	h.clearCookie(writer, loginCookie)
	h.setSessionCookie(writer, completion.Token, completion.ExpiresAt)
	http.Redirect(writer, request, "/", http.StatusSeeOther)
}

// Logout ends a browser session. Deliberately not gated: a session the gate
// will no longer admit is exactly the one whose cookie has to be cleared.
func (h *Handler) Logout(writer http.ResponseWriter, request *http.Request) {
	if !h.sameOrigin(request) {
		h.error(writer, http.StatusForbidden, "forbidden", "request origin is not permitted")

		return
	}
	if cookie, err := request.Cookie(sessionCookie); err == nil {
		// Best effort: the cookie is cleared either way, so a store that cannot
		// answer must not leave the caller apparently still signed in.
		if revokeErr := h.sessions.Revoke(request.Context(), cookie.Value); revokeErr != nil {
			slog.Warn("sign-out incomplete", "reason", "revoke_failed")
		}
	}
	h.clearCookie(writer, sessionCookie)
	writer.WriteHeader(http.StatusNoContent)
}

// pageValues is what a sign-in document is rendered with.
type pageValues struct {
	Message string
	Subject string
}

func (h *Handler) page(writer http.ResponseWriter, status int, name string, values any) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(status)
	if err := h.pages.ExecuteTemplate(writer, name, values); err != nil {
		slog.Error("rendering a sign-in page", "page", name, "error", err)
	}
}
