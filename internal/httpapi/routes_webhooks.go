package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
)

// eventWorkoutSummary is the one Wahoo event this receiver acts on. Wahoo is
// free to add others, which are accepted and ignored rather than refused.
const eventWorkoutSummary = "workout_summary"

// wahooWebhookEvent is the part of a notification this service reads. The
// workout it carries is never decoded: the poll reads the ride from Wahoo itself.
//
//nolint:tagliatelle // Wahoo's webhook body uses snake_case.
type wahooWebhookEvent struct {
	EventType    string `json:"event_type"`
	WebhookToken string `json:"webhook_token"`
	User         struct {
		ID int64 `json:"id"`
	} `json:"user"`
}

// ReceiveWahooWebhook starts one rider's activity poll ahead of its schedule.
// The body's shared token is its whole authentication and its user names the
// rider. Wahoo retries anything but 200 for three days, so an unknown user and
// an unhandled event kind are both absorbed as 200 with an empty body.
func (h *Handler) ReceiveWahooWebhook(writer http.ResponseWriter, request *http.Request) {
	// No verifier means no receiver, not one that invites guesses at the token.
	if h.webhookTokens == nil {
		h.notFound(writer)

		return
	}
	var event wahooWebhookEvent
	if err := json.NewDecoder(request.Body).Decode(&event); err != nil {
		h.error(writer, http.StatusBadRequest, "invalid_request", "the request body is not a webhook notification")

		return
	}
	verified, err := h.webhookTokens.VerifyWahooWebhookToken(request.Context(), event.WebhookToken)
	if err != nil {
		h.unavailable(writer)

		return
	}
	if !verified {
		// The presented value never reaches a log.
		slog.Warn("wahoo webhook refused", "reason", "token_mismatch")
		writer.WriteHeader(http.StatusUnauthorized)

		return
	}
	if event.EventType != eventWorkoutSummary {
		writer.WriteHeader(http.StatusOK)

		return
	}
	target, found, err := h.state.TargetByWahooUser(
		request.Context(), strconv.FormatInt(event.User.ID, 10))
	if err != nil {
		h.unavailable(writer)

		return
	}
	if !found {
		slog.Info("wahoo webhook ignored", "reason", "unknown_user")
		writer.WriteHeader(http.StatusOK)

		return
	}
	// A refused start means a poll already holds the activities resource.
	started := h.tasks.Run(TaskActivityPoll, target)
	slog.Info("wahoo webhook accepted", "target", target, "started", started)
	writer.WriteHeader(http.StatusOK)
}
