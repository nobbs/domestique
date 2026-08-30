package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nobbs/domestique/internal/httpapi"
	"github.com/nobbs/domestique/internal/komoot"
	"github.com/nobbs/domestique/internal/ridemodel"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/runtimeconfig"
	"github.com/nobbs/domestique/internal/sqlite"
	syncservice "github.com/nobbs/domestique/internal/sync"
	"github.com/nobbs/domestique/internal/veloplanner"
	"github.com/nobbs/domestique/internal/wahoo"
)

// oauthCallbackPath is where this service receives a Wahoo authorization. It is
// fixed rather than configurable because the Wahoo application registers one
// redirect URL and this binary serves exactly this path.
const oauthCallbackPath = "/oauth/wahoo/callback"

// errNotConfigured reports that the settings an upstream client is built from
// have not been entered yet. It is deliberately not an upstream failure: a
// service that has never been configured is doing exactly what it was deployed
// to do, and the settings page is where it is fixed.
var errNotConfigured = errors.New("the Wahoo application is not configured yet")

// wahooProvider hands out the Wahoo client the settings in force describe. The
// client carries the request budget observed from Wahoo's own responses, so it is
// rebuilt only when those settings change: a fresh one believes it has a full
// daily quota and spends real requests finding out otherwise.
type wahooProvider struct {
	settings    *runtimeconfig.Current
	client      *wahoo.Client
	redirectURL string
	built       wahooApplication
	generation  uint64
	mutex       sync.Mutex
}

// wahooApplication is the whole of what a client is built from, compared so
// that an edit to a setting the client never reads keeps the one already built.
// The secret is held as a digest because the only question ever asked of it
// here is whether it is still the one the client holds.
type wahooApplication struct {
	apiBaseURL   string
	oauthBaseURL string
	clientID     string
	secret       [sha256.Size]byte
}

// newWahooProvider builds the provider for one browser origin. The redirect URL
// is derived rather than configured: it is this service's own callback path on
// the origin a browser reaches it at, which is the only URL Wahoo may send an
// authorization back to.
func newWahooProvider(settings *runtimeconfig.Current, browserOriginURL string) *wahooProvider {
	return &wahooProvider{settings: settings, redirectURL: browserOriginURL + oauthCallbackPath}
}

func (p *wahooProvider) current() (*wahoo.Client, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	generation := p.settings.Generation()
	if p.client != nil && p.generation == generation {
		return p.client, nil
	}
	values := p.settings.Values().Wahoo
	secret := p.settings.Secret(runtimeconfig.SecretWahooClientSecret)
	if values.APIBaseURL == "" || values.OAuthBaseURL == "" || values.ClientID == "" ||
		!secret.IsSet() {
		return nil, errNotConfigured
	}
	application := wahooApplication{
		apiBaseURL:   values.APIBaseURL,
		oauthBaseURL: values.OAuthBaseURL,
		clientID:     values.ClientID,
		secret:       sha256.Sum256(secret.Bytes()),
	}
	if p.client != nil && p.built == application {
		p.generation = generation

		return p.client, nil
	}
	client, err := wahoo.New(&wahoo.Options{
		APIBaseURL:   values.APIBaseURL,
		OAuthBaseURL: values.OAuthBaseURL,
		ClientID:     values.ClientID,
		RedirectURL:  p.redirectURL,
		ClientSecret: secret.Bytes(),
	})
	if err != nil {
		return nil, fmt.Errorf("creating Wahoo client: %w", err)
	}
	p.client, p.built, p.generation = client, application, generation

	return client, nil
}

// targetIDs are the destination slots a run may reconcile: none at all until the
// Wahoo application itself is configured, so an incomplete setup makes runs
// report they are not ready rather than fail against an application that does
// not exist.
func (p *wahooProvider) targetIDs() []string {
	if _, err := p.current(); err != nil {
		return nil
	}

	return p.settings.Values().Wahoo.Targets
}

func (p *wahooProvider) AuthorizationURL(state string) (string, error) {
	client, err := p.current()
	if err != nil {
		return "", err
	}

	return client.AuthorizationURL(state) //nolint:wrapcheck // forwarding to the client this holds
}

func (p *wahooProvider) ExchangeAuthorizationCode(ctx context.Context, code string) (accessToken, refreshToken string, err error) {
	client, err := p.current()
	if err != nil {
		return "", "", err
	}

	return client.ExchangeAuthorizationCode(ctx, code) //nolint:wrapcheck // forwarding to the client this holds
}

func (p *wahooProvider) AuthenticatedUser(ctx context.Context, accessToken string) (string, error) {
	client, err := p.current()
	if err != nil {
		return "", err
	}

	return client.AuthenticatedUser(ctx, accessToken) //nolint:wrapcheck // forwarding to the client this holds
}

func (p *wahooProvider) RefreshAccessToken(ctx context.Context, refreshToken string) (accessToken, replacementRefreshToken string, err error) {
	client, err := p.current()
	if err != nil {
		return "", "", err
	}

	return client.RefreshAccessToken(ctx, refreshToken) //nolint:wrapcheck // forwarding to the client this holds
}

func (p *wahooProvider) ListOwnedRoutes(ctx context.Context, accessToken string) (map[string]int64, error) {
	client, err := p.current()
	if err != nil {
		return nil, err
	}

	return client.ListOwnedRoutes(ctx, accessToken) //nolint:wrapcheck // forwarding to the client this holds
}

func (p *wahooProvider) DeleteOwnedRoutes(ctx context.Context, accessToken string) (int, error) {
	client, err := p.current()
	if err != nil {
		return 0, err
	}

	return client.DeleteOwnedRoutes(ctx, accessToken) //nolint:wrapcheck // forwarding to the client this holds
}

func (p *wahooProvider) CreateRoute(ctx context.Context, accessToken string, stage *route.Route, fitData []byte) (routeID int64, err error) {
	client, err := p.current()
	if err != nil {
		return 0, err
	}

	return client.CreateRoute(ctx, accessToken, stage, fitData) //nolint:wrapcheck // forwarding to the client this holds
}

func (p *wahooProvider) UpdateRoute(ctx context.Context, routeID int64, accessToken string, stage *route.Route, fitData []byte) (updatedRouteID int64, err error) {
	client, err := p.current()
	if err != nil {
		return 0, err
	}

	return client.UpdateRoute(ctx, routeID, accessToken, stage, fitData) //nolint:wrapcheck // forwarding to the client this holds
}

func (p *wahooProvider) DeleteRoute(ctx context.Context, routeID int64, accessToken string) error {
	client, err := p.current()
	if err != nil {
		return err
	}

	return client.DeleteRoute(ctx, routeID, accessToken) //nolint:wrapcheck // forwarding to the client this holds
}

// IsUnauthorized classifies an error this provider returned. It answers without
// a client, because the sentinel belongs to the package rather than to any one
// client built from it.
func (p *wahooProvider) IsUnauthorized(err error) bool {
	return errors.Is(err, wahoo.ErrUnauthorized)
}

// RateLimit reports the budget the current client observed. An unconfigured
// service has spent nothing and knows nothing.
func (p *wahooProvider) RateLimit() (remaining int, resetAt time.Time, ok bool) {
	client, err := p.current()
	if err != nil {
		return 0, time.Time{}, false
	}

	return client.RateLimit()
}

// sources builds the library clients a run reads, in configured order. Built per
// call because neither client keeps a session between inventories. A source whose
// credentials are not entered is not skipped: reading part of a library and
// calling it the whole inventory is what the deletion gate exists to prevent.
func sources(settings *runtimeconfig.Current) ([]syncservice.Source, error) {
	configured := settings.Values().Sources
	built := make([]syncservice.Source, 0, len(configured))
	for _, source := range configured {
		emailName, passwordName, known := runtimeconfig.SourceSecretNames(source.Provider)
		if !known {
			return nil, fmt.Errorf("unknown source provider %q", source.Provider)
		}
		email := settings.Secret(emailName).Bytes()
		password := settings.Secret(passwordName).Bytes()
		if len(email) == 0 || len(password) == 0 {
			return nil, fmt.Errorf("%s credentials are not configured yet", source.Provider)
		}
		client, err := newSource(source, email, password)
		if err != nil {
			return nil, err
		}
		built = append(built, client)
	}

	return built, nil
}

func newSource(source runtimeconfig.Source, email, password []byte) (syncservice.Source, error) {
	switch source.Provider {
	case route.ProviderVeloPlanner:
		client, err := veloplanner.New(&veloplanner.Options{BaseURL: source.BaseURL, Email: email, Password: password})
		if err != nil {
			return nil, fmt.Errorf("creating VeloPlanner client: %w", err)
		}

		return client, nil
	case route.ProviderKomoot:
		client, err := komoot.New(&komoot.Options{BaseURL: source.BaseURL, Email: email, Password: password})
		if err != nil {
			return nil, fmt.Errorf("creating Komoot client: %w", err)
		}

		return client, nil
	}

	return nil, fmt.Errorf("unknown source provider %q", source.Provider)
}

// rideModelProvider predicts moving time from the coefficient file in force,
// reloading it when an operator points the setting elsewhere. No file configured
// means no stage carries a guessed rider figure, and a file that will not load is
// a failed prediction rather than a substituted one.
type rideModelProvider struct {
	store      *sqlite.Store
	predictor  *ridemodel.Predictor
	validation *httpapi.RideModelValidation
	path       string
	mutex      sync.Mutex
	loaded     bool
}

func newRideModelProvider(store *sqlite.Store) *rideModelProvider {
	return &rideModelProvider{store: store}
}

// reload resolves the configured file, replacing what is loaded when it moved.
// Predictions cached against a different file are dropped as part of the swap.
func (p *rideModelProvider) reload(ctx context.Context, settings *runtimeconfig.Current) error {
	path := settings.Values().RideModel.CoefficientsFile

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.loaded && p.path == path {
		return nil
	}

	var fingerprint string
	var predictor *ridemodel.Predictor
	var validation *httpapi.RideModelValidation
	if path != "" {
		coefficients, err := ridemodel.Load(path)
		if err != nil {
			return fmt.Errorf("loading ride model coefficients: %w", err)
		}
		fingerprint = coefficients.Fingerprint
		predictor = ridemodel.NewPredictor(p.store, p.store, coefficients)
		if coefficients.HasValidation() {
			validation = &httpapi.RideModelValidation{
				BiasPercent:    coefficients.BiasPercent,
				MAEPercent:     coefficients.MAEPercent,
				P90Percent:     coefficients.P90Percent,
				EvaluatedRides: coefficients.EvaluatedRides,
			}
		}
	}
	if err := p.store.PruneStageDurationsWithDifferentFingerprint(ctx, fingerprint); err != nil {
		return fmt.Errorf("pruning stale ride model predictions: %w", err)
	}
	// A stage cannot be failing a pass nothing is asking for, and one measured
	// against a file that has been replaced is not failing the new one either.
	if err := p.store.ClearStageDurationFailures(ctx); err != nil {
		return fmt.Errorf("clearing stale ride model failures: %w", err)
	}
	p.predictor, p.validation, p.path, p.loaded = predictor, validation, path, true

	return nil
}

func (p *rideModelProvider) current() *ridemodel.Predictor {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return p.predictor
}

// validationView is what the stage endpoint reports about the loaded model's
// measured accuracy, or nothing when no model is loaded or it carries none.
func (p *rideModelProvider) validationView() *httpapi.RideModelValidation {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return p.validation
}

// predictorFor binds a provider to the settings it reloads from, so that
// synchronization keeps seeing nothing but Predict.
func predictorFor(provider *rideModelProvider, settings *runtimeconfig.Current) syncservice.Predictor {
	return predictorFunc(func(ctx context.Context, stages []route.Route) (predicted, failed int, err error) {
		if reloadErr := provider.reload(ctx, settings); reloadErr != nil {
			return 0, 0, reloadErr
		}
		predictor := provider.current()
		if predictor == nil {
			return 0, 0, nil
		}

		return predictor.Predict(ctx, stages)
	})
}

type predictorFunc func(ctx context.Context, stages []route.Route) (predicted, failed int, err error)

func (f predictorFunc) Predict(ctx context.Context, stages []route.Route) (predicted, failed int, err error) {
	return f(ctx, stages)
}
