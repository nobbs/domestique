package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nobbs/domestique/internal/runtimeconfig"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testWebhookToken = "the-configured-webhook-token"
	testWahooUserID  = "44556677"
	testWorkoutID    = 56519
)

// fakeWebhookTokens verifies against one configured token, or fails, which is
// the whole of what the receiver is ever told about a credential.
type fakeWebhookTokens struct {
	err        error
	configured string
	presented  []string
}

func (v *fakeWebhookTokens) VerifyWahooWebhookToken(_ context.Context, presented string) (bool, error) {
	v.presented = append(v.presented, presented)
	if v.err != nil {
		return false, v.err
	}

	return v.configured != "" && presented == v.configured, nil
}

// newWebhookHandler builds a handler whose receiver is on, over state that
// knows one Wahoo identity, and returns the doubles a test reads back.
func newWebhookHandler(t *testing.T) (*Handler, *fakeTasks, *fakeWebhookTokens, *fakeState) {
	t.Helper()

	tasks := &fakeTasks{}
	verifier := &fakeWebhookTokens{configured: testWebhookToken}
	state := &fakeState{wahooUsers: map[string]string{testWahooUserID: testSubject}}
	settings := settingsWith(testBasemaps())
	settings.secrets = map[runtimeconfig.SecretName]runtimeconfig.Secret{
		runtimeconfig.SecretWahooWebhookToken: runtimeconfig.NewSecret([]byte(testWebhookToken)),
	}
	handler, err := New(
		&Options{
			schemaCache:      testSchemaCache,
			Alerts:           &fakeAlerts{},
			Tasks:            tasks,
			Settings:         settings,
			Sessions:         newFakeSessions(),
			BrowserOriginURL: testBrowserOriginURL,
			WebhookTokens:    verifier,
		},
		&fakeOAuth{}, state, &fakeSync{accepted: true}, &fakeAssets{}, &fakeWeather{}, &fakeWeatherGrid{},
	)
	require.NoError(t, err, "New()")

	return handler, tasks, verifier, state
}

// webhookRequest is a delivery as Wahoo sends one: no cookie, no Origin, and
// the token in the body.
func webhookRequest(t *testing.T, body string) *http.Request {
	t.Helper()

	request := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/webhooks/wahoo", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	return request
}

// webhookBody is a workout notification for one Wahoo user.
func webhookBody(t *testing.T, token, eventType, userID string) string {
	t.Helper()

	body, err := json.Marshal(map[string]any{
		"event_type":    eventType,
		"webhook_token": token,
		"user":          map[string]string{"id": userID},
		"workout_summary": map[string]any{
			"id": 1, "distance_accum": "12345.0", "ascent_accum": "210.0",
			// The workout's own id, which is not the summary's id beside it.
			"workout": map[string]any{"id": testWorkoutID},
		},
	})
	require.NoError(t, err, "building the notification")
	// The user id is an integer on the wire; the map above keeps it a string so
	// the fixture reads as one value, so it is unquoted here.
	return strings.Replace(string(body), `"id":"`+userID+`"`, `"id":`+userID, 1)
}

// captureLogs redirects the package logger for one test and returns what it
// wrote, so an assertion can be made about what a refusal did not record.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	var recorded bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&recorded, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	return &recorded
}

// The delivery this receiver exists for: the configured token, a rider this
// deployment knows, and that rider's notified workout handed off to be read
// from Wahoo. No session cookie is carried, because Wahoo has none.
func TestWahooWebhookRecordsTheNotifiedWorkout(t *testing.T) {
	handler, tasks, _, _ := newWebhookHandler(t)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, webhookRequest(t,
		webhookBody(t, testWebhookToken, eventWorkoutSummary, testWahooUserID)))

	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, []startedTask{{
		name: TaskActivityRecord, argument: ActivityRecordArgument(testSubject, testWorkoutID),
	}}, tasks.started)
}

// Wahoo's documented sample, with the summary and its nested workout, decodes
// to the three fields read: the rest is carried without being looked at.
func TestWahooWebhookAcceptsWahoosDocumentedShape(t *testing.T) {
	handler, tasks, _, _ := newWebhookHandler(t)
	body := fmt.Sprintf(`{
  "event_type": "workout_summary",
  "webhook_token": %q,
  "user": {"id": %s},
  "workout_summary": {
    "id": 8297, "ascent_accum": "450.00", "cadence_avg": "52.00",
    "distance_accum": "24909.71", "duration_active_accum": "179.00",
    "duration_paused_accum": "85.00", "duration_total_accum": "275.20",
    "created_at": "2018-10-23T20:43:50.000Z", "updated_at": "2018-10-23T20:43:50.000Z",
    "file": {"url": "https://server.example/4_Mile_Segment_.fit"},
    "workout": {
      "id": 56519, "starts": "2015-08-12T09:00:00.000Z", "minutes": 12,
      "name": "Friday Fun", "plan_id": null, "workout_token": "123", "workout_type_id": 40
    }
  }
}`, testWebhookToken, testWahooUserID)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, webhookRequest(t, body))

	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, []startedTask{{
		name: TaskActivityRecord, argument: ActivityRecordArgument(testSubject, testWorkoutID),
	}}, tasks.started)
}

// A refused start is what a poll already reading that rider's rides looks like,
// so it is still a 200: answering otherwise buys four Wahoo retries of work
// that is already happening.
func TestWahooWebhookAnswersOKWhenTheRunWasRefused(t *testing.T) {
	handler, tasks, _, _ := newWebhookHandler(t)
	tasks.refuse = true

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, webhookRequest(t,
		webhookBody(t, testWebhookToken, eventWorkoutSummary, testWahooUserID)))

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Len(t, tasks.asked, 1, "the attempt was never made")
	assert.Empty(t, tasks.started)
}

// A wrong token is refused with nothing in the body and nothing in the log: a
// presented value that reached a log is a credential written to disk.
func TestWahooWebhookRefusesAWrongTokenWithoutRecordingIt(t *testing.T) {
	recorded := captureLogs(t)
	handler, tasks, _, _ := newWebhookHandler(t)
	const presented = "a-guessed-webhook-token"

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, webhookRequest(t,
		webhookBody(t, presented, eventWorkoutSummary, testWahooUserID)))

	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.Empty(t, response.Body.String(), "a refusal must carry no body")
	assert.Empty(t, tasks.asked, "a refused delivery started work")
	assert.NotContains(t, recorded.String(), presented, "the presented token reached the log")
	assert.Contains(t, recorded.String(), "token_mismatch")
}

// A body with no token at all is the same refusal: the receiver has one way in.
func TestWahooWebhookRefusesAMissingToken(t *testing.T) {
	handler, tasks, _, _ := newWebhookHandler(t)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, webhookRequest(t,
		`{"event_type":"workout_summary","user":{"id":`+testWahooUserID+`}}`))

	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.Empty(t, tasks.asked)
}

// A Wahoo user nobody has connected is answered exactly like one who has, so
// the receiver cannot be used to enumerate which riders this deployment holds.
func TestWahooWebhookIgnoresAnUnknownUser(t *testing.T) {
	handler, tasks, _, _ := newWebhookHandler(t)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, webhookRequest(t,
		webhookBody(t, testWebhookToken, eventWorkoutSummary, "99887766")))

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Empty(t, response.Body.String())
	assert.Empty(t, tasks.asked, "an unknown user started work")
}

// A notification naming no workout buys nothing to verify, and a poll of the
// whole account is not what it paid for, so it is accepted and ignored.
func TestWahooWebhookIgnoresANotificationWithNoWorkout(t *testing.T) {
	handler, tasks, _, _ := newWebhookHandler(t)
	body := `{"event_type":"workout_summary","webhook_token":"` + testWebhookToken +
		`","user":{"id":` + testWahooUserID + `},"workout_summary":{"id":1}}`

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, webhookRequest(t, body))

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Empty(t, response.Body.String())
	assert.Empty(t, tasks.asked, "a notification naming no workout started work")
}

// Wahoo may add event kinds. One this service does not act on is accepted and
// ignored rather than refused into four retries.
func TestWahooWebhookIgnoresAnotherEventType(t *testing.T) {
	handler, tasks, _, _ := newWebhookHandler(t)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, webhookRequest(t,
		webhookBody(t, testWebhookToken, "workout_deleted", testWahooUserID)))

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Empty(t, tasks.asked)
}

// The body is read before the token can be, so a body that is not a
// notification is refused before anything else is decided about it.
func TestWahooWebhookRefusesAMalformedBody(t *testing.T) {
	handler, tasks, verifier, _ := newWebhookHandler(t)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, webhookRequest(t, `{"event_type":`))

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Contains(t, response.Body.String(), "invalid_request")
	assert.Empty(t, verifier.presented, "a body that never parsed reached the verifier")
	assert.Empty(t, tasks.asked)
}

// A verifier that cannot answer leaves the delivery unhandled, so it is refused
// rather than silently accepted — Wahoo's retries are the recovery.
func TestWahooWebhookRefusesWhenTheVerifierFails(t *testing.T) {
	handler, tasks, verifier, _ := newWebhookHandler(t)
	verifier.err = errors.New("the settings could not be read")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, webhookRequest(t,
		webhookBody(t, testWebhookToken, eventWorkoutSummary, testWahooUserID)))

	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.Empty(t, tasks.asked)
}

// A state read that fails is the same: the rider may well be known, so the
// delivery is refused and retried rather than dropped as an unknown user.
func TestWahooWebhookRefusesWhenTheTargetLookupFails(t *testing.T) {
	handler, tasks, _, state := newWebhookHandler(t)
	state.wahooUserErr = errors.New("the state database is unavailable")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, webhookRequest(t,
		webhookBody(t, testWebhookToken, eventWorkoutSummary, testWahooUserID)))

	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.Empty(t, tasks.asked)
}

// Only POST is registered, so every other method falls to the unmatched-path
// answer this service gives any path it does not serve, and never reaches the
// receiver.
func TestWahooWebhookRefusesEveryOtherMethod(t *testing.T) {
	handler, tasks, _, _ := newWebhookHandler(t)

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequestWithContext(
				t.Context(), method, "/webhooks/wahoo", http.NoBody))

			assert.Equal(t, http.StatusNotFound, response.Code)
			assert.Empty(t, tasks.asked)
		})
	}
}

// A deployment that has registered no webhook has no receiver: the route is
// answered not found rather than refusing every caller and inviting a guess.
func TestWahooWebhookIsNotFoundWithoutAVerifier(t *testing.T) {
	handler := newTestHandler(t)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, webhookRequest(t,
		webhookBody(t, testWebhookToken, eventWorkoutSummary, testWahooUserID)))

	assert.Equal(t, http.StatusNotFound, response.Code)
}

// Until an operator stores the token there is nothing to verify against, so
// there is no receiver either: not found, never a 401 that invites a guess.
func TestWahooWebhookIsNotFoundUntilATokenIsStored(t *testing.T) {
	handler, tasks, verifier, _ := newWebhookHandler(t)
	settings, ok := handler.settings.(*staticSettings)
	require.True(t, ok, "settings double")
	settings.secrets = nil

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, webhookRequest(t,
		webhookBody(t, testWebhookToken, eventWorkoutSummary, testWahooUserID)))

	assert.Equal(t, http.StatusNotFound, response.Code)
	assert.Empty(t, tasks.started)
	assert.Empty(t, verifier.presented, "the verifier was asked with nothing stored")
}

// The receiver reads a bounded body like every other route, so a delivery that
// is not a workout summary at all cannot be used to make this service read.
func TestWahooWebhookBoundsTheBody(t *testing.T) {
	handler, tasks, _, _ := newWebhookHandler(t)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, webhookRequest(t,
		`{"padding":"`+strings.Repeat("a", int(maximumWebhookBytes)+1)+`"}`))

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Empty(t, tasks.asked)
}

// The receiver is deliberately outside the OpenAPI contract: that document
// describes the session-gated API, and this route is neither gated nor under
// /v1/.
func TestWahooWebhookIsNotInTheContract(t *testing.T) {
	document := loadOpenAPIContract(t)

	assert.NotContains(t, document.Paths, "/webhooks/wahoo")
}
