package sync

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/route"
)

var errDeviceUnauthorized = errors.New("device: unauthorized")

type fakeDeviceAPI struct {
	signInErr   error
	routesErr   map[string]error
	writeErr    map[int64]error
	validTokens map[string]bool
	written     map[int64]string
	routes      []DeviceRoute
	signIns     int
}

func (f *fakeDeviceAPI) SignIn(_ context.Context, email, password []byte) (string, error) {
	f.signIns++
	if f.signInErr != nil {
		return "", f.signInErr
	}
	token := "fresh:" + string(email) + ":" + string(password)
	f.validTokens[token] = true

	return token, nil
}

func (f *fakeDeviceAPI) Routes(_ context.Context, token string) ([]DeviceRoute, error) {
	if err := f.routesErr[token]; err != nil {
		return nil, err
	}
	if !f.validTokens[token] {
		return nil, errDeviceUnauthorized
	}

	return f.routes, nil
}

func (f *fakeDeviceAPI) SetProviderID(_ context.Context, _ string, routeID int64, value string) error {
	if err := f.writeErr[routeID]; err != nil {
		return err
	}
	f.written[routeID] = value

	return nil
}

func (f *fakeDeviceAPI) IsUnauthorized(err error) bool { return errors.Is(err, errDeviceUnauthorized) }

type fakeLabelState struct {
	stateErr       error
	credentialsErr error
	setErr         error
	owners         map[string]string
	credentials    map[string]map[rider.CredentialName]rider.Credential
}

func (f *fakeLabelState) TargetOwner(_ context.Context, targetID string) (string, error) {
	return f.owners[targetID], f.stateErr
}

func (f *fakeLabelState) RiderCredentials(_ context.Context, subject string) (map[rider.CredentialName]rider.Credential, error) {
	return f.credentials[subject], f.credentialsErr
}

func (f *fakeLabelState) SetRiderCredentials(
	_ context.Context, subject string, credentials map[rider.CredentialName]rider.Credential,
) error {
	if f.setErr != nil {
		return f.setErr
	}
	for name, credential := range credentials {
		if credential.IsSet() {
			f.credentials[subject][name] = credential
		} else {
			delete(f.credentials[subject], name)
		}
	}

	return nil
}

func ownedExternalID(t *testing.T, sourceRouteID int64) string {
	t.Helper()
	stage := testStage(t, sourceRouteID, 1, "revision", "hash")

	return stage.Key().ExternalID()
}

func newLabelFixture(t *testing.T, credentials map[rider.CredentialName]rider.Credential) (*DeviceLabeler, *fakeDeviceAPI, *fakeLabelState) {
	t.Helper()
	api := &fakeDeviceAPI{
		validTokens: map[string]bool{"stored": true},
		written:     map[int64]string{},
		routes: []DeviceRoute{
			{ID: 11, ExternalID: ownedExternalID(t, 1)},
			{ID: 12, ExternalID: ownedExternalID(t, 2), ProviderID: "local-kept"},
			{ID: 13, ExternalID: "someone-else"},
			{ID: 14, ProviderID: "komoot"},
		},
	}
	state := &fakeLabelState{
		owners:      map[string]string{"a": "rider-a"},
		credentials: map[string]map[rider.CredentialName]rider.Credential{"rider-a": credentials},
	}
	labeler, err := NewDeviceLabeler(api, state)
	require.NoError(t, err)

	return labeler, api, state
}

func wahooCredentials(session string) map[rider.CredentialName]rider.Credential {
	credentials := map[rider.CredentialName]rider.Credential{
		rider.CredentialWahooEmail:    rider.NewCredential([]byte("rider@example.test")),
		rider.CredentialWahooPassword: rider.NewCredential([]byte("secret")),
	}
	if session != "" {
		credentials[rider.CredentialWahooSession] = rider.NewWahooSession(credentials, session)
	}

	return credentials
}

func TestLabelerFillsOnlyTheEmptyProviderIDOfOwnedRoutesWithTheStoredSession(t *testing.T) {
	labeler, api, _ := newLabelFixture(t, wahooCredentials("stored"))

	labeler.Label(t.Context(), "a")

	assert.Equal(t, map[int64]string{11: "11"}, api.written)
	assert.Zero(t, api.signIns, "a stored session that still works needs no sign-in")
}

func TestLabelerSignsInAfreshOnlyWhenTheStoredSessionIsRejected(t *testing.T) {
	labeler, api, state := newLabelFixture(t, wahooCredentials("expired"))

	labeler.Label(t.Context(), "a")

	assert.Equal(t, 1, api.signIns)
	assert.Equal(t, map[int64]string{11: "11"}, api.written)
	token, stored := rider.WahooSession(state.credentials["rider-a"])
	assert.True(t, stored, "the new session is kept for the next run")
	assert.Equal(t, "fresh:rider@example.test:secret", token)
}

func TestLabelerSignsInWhenNoSessionIsStored(t *testing.T) {
	labeler, api, state := newLabelFixture(t, wahooCredentials(""))

	labeler.Label(t.Context(), "a")

	assert.Equal(t, 1, api.signIns)
	assert.Equal(t, map[int64]string{11: "11"}, api.written)
	assert.True(t, state.credentials["rider-a"][rider.CredentialWahooSession].IsSet())
}

func TestLabelerMarksRefusedCredentialsAndStopsSigningIn(t *testing.T) {
	labeler, api, state := newLabelFixture(t, wahooCredentials("expired"))
	api.signInErr = errDeviceUnauthorized

	labeler.Label(t.Context(), "a")
	labeler.Label(t.Context(), "a")

	assert.Equal(t, 1, api.signIns, "a refused password is not tried again until the rider saves a new one")
	assert.Empty(t, api.written)
	assert.True(t, state.credentials["rider-a"][rider.CredentialWahooRefused].IsSet())
	assert.NotContains(t, state.credentials["rider-a"], rider.CredentialWahooSession)
}

func TestLabelerLeavesTheSessionAloneWhenSignInFailsForAnotherReason(t *testing.T) {
	labeler, api, state := newLabelFixture(t, wahooCredentials("expired"))
	api.signInErr = errors.New("device: HTTP 503")

	labeler.Label(t.Context(), "a")

	assert.Empty(t, api.written)
	assert.NotContains(t, state.credentials["rider-a"], rider.CredentialWahooRefused)
}

func TestLabelerStopsWhenTheSessionIsRejectedMidway(t *testing.T) {
	labeler, api, _ := newLabelFixture(t, wahooCredentials("stored"))
	api.routes = append(api.routes, DeviceRoute{ID: 15, ExternalID: ownedExternalID(t, 3)})
	api.writeErr = map[int64]error{11: errDeviceUnauthorized}

	labeler.Label(t.Context(), "a")

	assert.Empty(t, api.written, "no write follows a rejected session")
}

func TestLabelerContinuesPastOneRefusedWrite(t *testing.T) {
	labeler, api, _ := newLabelFixture(t, wahooCredentials("stored"))
	api.routes = append(api.routes, DeviceRoute{ID: 15, ExternalID: ownedExternalID(t, 3)})
	api.writeErr = map[int64]error{11: errors.New("device: HTTP 422")}

	labeler.Label(t.Context(), "a")

	assert.Equal(t, map[int64]string{15: "15"}, api.written)
}

func TestLabelerSkipsARiderWithoutWahooCredentials(t *testing.T) {
	for name, credentials := range map[string]map[rider.CredentialName]rider.Credential{
		"none":          {},
		"no password":   {rider.CredentialWahooEmail: rider.NewCredential([]byte("rider@example.test"))},
		"zwift only":    {rider.CredentialZwiftEmail: rider.NewCredential([]byte("z")), rider.CredentialZwiftPassword: rider.NewCredential([]byte("z"))},
		"after refusal": {rider.CredentialWahooRefused: rider.NewCredential([]byte("1"))},
	} {
		t.Run(name, func(t *testing.T) {
			labeler, api, _ := newLabelFixture(t, credentials)

			labeler.Label(t.Context(), "a")

			assert.Zero(t, api.signIns)
			assert.Empty(t, api.written)
		})
	}
}

func TestLabelerSkipsATargetWithoutAnOwnerOrReadableState(t *testing.T) {
	labeler, api, state := newLabelFixture(t, wahooCredentials("stored"))
	labeler.Label(t.Context(), "unowned")
	state.stateErr = errors.New("unreadable")
	labeler.Label(t.Context(), "a")

	assert.Empty(t, api.written)
}

type recordingLabeler struct{ targets []string }

func (r *recordingLabeler) Label(_ context.Context, targetID string) {
	r.targets = append(r.targets, targetID)
}

func TestServiceLabelsEachTargetAfterReconcilingItWhateverTheOutcome(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a", "b")
	state.trusted = []route.Route{desired}
	target := newFakeTarget()
	target.listErr = errDestination
	labeler := &recordingLabeler{}
	options := syncOptions(false, []Source{&fakeSource{}}, "a", "b")
	options.Labeler = labeler
	service, err := New(options, state, identityProcessor{}, &fakeEncoder{}, target, nil, nil)
	require.NoError(t, err)

	assert.Equal(t, OutcomeFailed, service.RunTargets(t.Context()).Outcome)
	service.RunTarget(t.Context(), "b")

	assert.Equal(t, []string{"a", "b", "b"}, labeler.targets)
}

func TestNewDeviceLabelerRequiresItsDependencies(t *testing.T) {
	_, err := NewDeviceLabeler(nil, nil)
	require.Error(t, err)
}

func TestLabelerWritesNothingWhenTheStoreOrListingFails(t *testing.T) {
	freshToken := "fresh:rider@example.test:secret"
	for name, arrange := range map[string]func(*fakeDeviceAPI, *fakeLabelState){
		"credentials unreadable": func(_ *fakeDeviceAPI, state *fakeLabelState) {
			state.credentialsErr = errors.New("unreadable")
		},
		"stored listing fails": func(api *fakeDeviceAPI, _ *fakeLabelState) {
			api.routesErr = map[string]error{"stored": errors.New("device: HTTP 503")}
		},
		"fresh listing fails": func(api *fakeDeviceAPI, _ *fakeLabelState) {
			api.validTokens = map[string]bool{}
			api.routesErr = map[string]error{freshToken: errors.New("device: HTTP 503")}
		},
		"new session unstorable": func(api *fakeDeviceAPI, state *fakeLabelState) {
			api.validTokens = map[string]bool{}
			state.setErr = errors.New("unwritable")
		},
		"refusal unstorable": func(api *fakeDeviceAPI, state *fakeLabelState) {
			api.validTokens = map[string]bool{}
			api.signInErr = errDeviceUnauthorized
			state.setErr = errors.New("unwritable")
		},
	} {
		t.Run(name, func(t *testing.T) {
			labeler, api, state := newLabelFixture(t, wahooCredentials("stored"))
			arrange(api, state)

			labeler.Label(t.Context(), "a")

			assert.Empty(t, api.written)
		})
	}
}

// A session or refusal stored for a pair the rider has since replaced, by a
// run racing their save, is ignored rather than trusted.
func TestLabelerIgnoresASessionOrRefusalOfAnotherPair(t *testing.T) {
	old := wahooCredentials("")
	old[rider.CredentialWahooPassword] = rider.NewCredential([]byte("old"))
	current := wahooCredentials("")
	current[rider.CredentialWahooSession] = rider.NewWahooSession(old, "stored")
	current[rider.CredentialWahooRefused] = rider.NewWahooRefusal(old)
	labeler, api, _ := newLabelFixture(t, current)

	labeler.Label(t.Context(), "a")

	assert.Equal(t, 1, api.signIns, "the old pair's session is not reused")
	assert.Equal(t, map[int64]string{11: "11"}, api.written, "the old pair's refusal does not stop the new one")
}

func TestLabelerWritesAtMostTheRunCap(t *testing.T) {
	labeler, api, _ := newLabelFixture(t, wahooCredentials("stored"))
	api.routes = nil
	for index := range maxLabelsPerRun + 5 {
		api.routes = append(api.routes, DeviceRoute{ID: int64(100 + index), ExternalID: ownedExternalID(t, int64(100+index))})
	}

	labeler.Label(t.Context(), "a")

	assert.Len(t, api.written, maxLabelsPerRun)
}
