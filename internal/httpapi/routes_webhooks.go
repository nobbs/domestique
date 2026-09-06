package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/nobbs/domestique/internal/runtimeconfig"
)

// eventWorkoutSummary is the one Wahoo event this receiver acts on. Wahoo is
// free to add others, which are accepted and ignored rather than refused.
const eventWorkoutSummary = "workout_summary"

// webhookWahooPath is the one path outside the identity gate besides sign-in.
const webhookWahooPath = "/webhooks/wahoo"

// wahooWebhookEvent is the part of a notification this service reads: who it is
// about and which workout it names. Nothing the workout summary says about that
// workout is read; the task reads the ride from Wahoo itself.
//
//nolint:tagliatelle // Wahoo's webhook body uses snake_case.
type wahooWebhookEvent struct {
	EventType    string `json:"event_type"`
	WebhookToken string `json:"webhook_token"`
	User         struct {
		ID int64 `json:"id"`
	} `json:"user"`
	// The workout's own id, nested inside the summary; the summary's id beside
	// it is the summary's, and names no workout.
	WorkoutSummary struct {
		Workout struct {
			ID int64 `json:"id"`
		} `json:"workout"`
	} `json:"workout_summary"`
}

// ReceiveWahooWebhook has one rider's notified workout read from Wahoo and
// stored. The body's shared token is its whole authentication, its user names
// the rider and its workout summary names the ride. Wahoo retries anything but
// 200 for three days, so an unknown user, a notification naming no workout and
// an unhandled event kind are all absorbed as 200 with an empty body.
func (h *Handler) ReceiveWahooWebhook(writer http.ResponseWriter, request *http.Request) {
	// No verifier, or no token stored yet, means no receiver rather than one that
	// invites guesses at the token; the settings are read per request, so
	// storing the token switches it on without a restart.
	if h.webhookTokens == nil || !h.settings.SecretIsSet(runtimeconfig.SecretWahooWebhookToken) {
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
	// Nothing to verify against the account, and a poll of the whole account is
	// not what this notification bought.
	if event.WorkoutSummary.Workout.ID <= 0 {
		slog.Info("wahoo webhook ignored", "reason", "no_workout")
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
	// A refused start means a poll or another notification holds the activities
	// resource; the schedule is the fallback for it.
	started := h.tasks.Run(TaskActivityRecord, ActivityRecordArgument(target, event.WorkoutSummary.Workout.ID))
	slog.Info("wahoo webhook accepted", "target", target, "started", started)
	writer.WriteHeader(http.StatusOK)
}
