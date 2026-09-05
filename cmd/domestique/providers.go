package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/auth0"
	"github.com/nobbs/domestique/internal/httpapi"
	"github.com/nobbs/domestique/internal/komoot"
	"github.com/nobbs/domestique/internal/ridemodel"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/runtimeconfig"
	"github.com/nobbs/domestique/internal/session"
	"github.com/nobbs/domestique/internal/sqlite"
	syncservice "github.com/nobbs/domestique/internal/sync"
	"github.com/nobbs/domestique/internal/veloplanner"
	"github.com/nobbs/domestique/internal/wahoo"
)

// oauthCallbackPath is where this service receives a Wahoo authorization. It is
// fixed rather than configurable because the Wahoo application registers one
// redirect URL and this binary serves exactly this path.
const oauthCallbackPath = "/oauth/wahoo/callback"

// signInCallbackPath is where the sign-in provider returns a browser. Fixed for
// the same reason: the Auth0 application registers one redirect URL.
const signInCallbackPath = "/auth/callback"

// signInProvider adapts the Auth0 client to session.Provider, which is kept to
// primitives so that package never imports this adapter.
type signInProvider struct{ client *auth0.Client }

func (p signInProvider) AuthorizationURL(ctx context.Context, state, nonce, codeVerifier string) (string, error) {
	return p.client.AuthorizationURL(ctx, state, nonce, codeVerifier) //nolint:wrapcheck // forwarding to the client this holds
}

func (p signInProvider) Exchange(
	ctx context.Context, code, codeVerifier, nonce string,
) (session.ExchangedIdentity, error) {
	identity, err := p.client.Exchange(ctx, code, codeVerifier, nonce)
	if err != nil {
		return session.ExchangedIdentity{}, err //nolint:wrapcheck // forwarding to the client this holds
	}

	return exchangedIdentityFrom(identity), nil
}

// exchangedIdentityFrom narrows an Auth0 identity to what session.Provider
// needs, split out from Exchange so the field mapping is directly testable
// without a live or fake token exchange.
func exchangedIdentityFrom(identity auth0.Identity) session.ExchangedIdentity {
	return session.ExchangedIdentity{
		Subject: identity.Subject, Email: identity.Email, Name: identity.Name,
		Access: identity.Access, Admin: identity.Admin,
	}
}

// targetIDsTimeout bounds the one local read targetIDs performs. A stuck read
// must not stall whatever task-scheduling path asked for the target list.
const targetIDsTimeout = 3 * time.Second

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
	store       *sqlite.Store
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
func newWahooProvider(settings *runtimeconfig.Current, store *sqlite.Store, browserOriginURL string) *wahooProvider {
	return &wahooProvider{settings: settings, store: store, redirectURL: browserOriginURL + oauthCallbackPath}
}

func (p *wahooProvider) current() (*wahoo.Client, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	snapshot := p.settings.Snapshot()
	generation := snapshot.Generation()
	if p.client != nil && p.generation == generation {
		return p.client, nil
	}
	values := snapshot.Values().Wahoo
	secret := snapshot.Secret(runtimeconfig.SecretWahooClientSecret)
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

// targetIDs are every self-service target's slot, unfiltered by owner: none
// at all until the Wahoo application itself is configured, so an incomplete
// setup makes runs report they are not ready rather than fail against an
// application that does not exist. Background reconciliation has no notion
// of ownership — it reconciles whatever rows exist, the same as it always
// has; only the HTTP surface a rider or admin actually sees is scoped.
func (p *wahooProvider) targetIDs() []string {
	if _, err := p.current(); err != nil {
		return nil
	}

	// Best effort: the sync service's own targetIDs has no error channel, so a
	// read failure here is reported as no targets — the "not ready" state a
	// caller already handles — but logged first, so it is not read as the
	// ordinary "nothing configured yet" case an operator need not act on. The
	// read is bounded so a stuck store cannot stall this call forever.
	ctx, cancel := context.WithTimeout(context.Background(), targetIDsTimeout)
	defer cancel()
	ids := []string{}
	if err := p.store.ForEachTarget(ctx, func(id, _, _ string) error {
		ids = append(ids, id)

		return nil
	}); err != nil {
		slog.Error("reading target IDs", "error", err)

		return nil
	}

	return ids
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

// ListActivities reads the rider's recorded activities in this service's own
// vocabulary, so the activity package never learns Wahoo's word for them.
func (p *wahooProvider) ListActivities(ctx context.Context, accessToken string) ([]activity.Listing, error) {
	client, err := p.current()
	if err != nil {
		return nil, err
	}
	workouts, err := client.ListWorkouts(ctx, accessToken)
	if err != nil {
		return nil, fmt.Errorf("listing Wahoo workouts: %w", err)
	}

	return activityListings(workouts), nil
}

// ActivityListingHead reads the account's first page of activities and how many
// it holds, which is what tells a poll whether the whole list is worth reading.
func (p *wahooProvider) ActivityListingHead(ctx context.Context, accessToken string) (listings []activity.Listing, total int, err error) {
	client, err := p.current()
	if err != nil {
		return nil, 0, err
	}
	workouts, total, err := client.WorkoutListingHead(ctx, accessToken)
	if err != nil {
		return nil, 0, fmt.Errorf("reading the Wahoo workout listing head: %w", err)
	}

	return activityListings(workouts), total, nil
}

// activityListings narrows Wahoo's workouts to what the activity package reads,
// split out so the field mapping is directly testable without a live client.
func activityListings(workouts []wahoo.Workout) []activity.Listing {
	listings := make([]activity.Listing, len(workouts))
	for index, workout := range workouts {
		listings[index] = activity.Listing{
			ID:         workout.ID,
			TypeID:     workout.WorkoutTypeID,
			LocationID: workout.WorkoutTypeLocationID,
			Starts:     workout.Starts,
		}
	}

	return listings
}

// DownloadActivityFIT reads the FIT file a stored summary names, from Wahoo's
// CDN. Where the URL sits in that document is Wahoo's shape, so it is read here.
func (p *wahooProvider) DownloadActivityFIT(ctx context.Context, summary activity.Summary) ([]byte, error) {
	client, err := p.current()
	if err != nil {
		return nil, err
	}
	fileURL, err := summaryFileURL(summary.Raw)
	if err != nil {
		return nil, err
	}
	data, err := client.DownloadWorkoutFIT(ctx, fileURL)
	if errors.Is(err, wahoo.ErrWorkoutFileRefused) {
		return nil, fmt.Errorf("%w: %w", activity.ErrNoActivityFile, err)
	}
	if err != nil {
		return nil, fmt.Errorf("downloading a Wahoo workout file: %w", err)
	}

	return data, nil
}

// summaryFileURL is the FIT file URL inside Wahoo's own summary document.
func summaryFileURL(raw []byte) (string, error) {
	var summary struct {
		File struct {
			URL string `json:"url"`
		} `json:"file"`
	}
	if err := json.Unmarshal(raw, &summary); err != nil {
		// The cause names a field or a token, never the file URL, so it may travel.
		return "", fmt.Errorf("%w: stored Wahoo workout summary could not be read as one: %w", activity.ErrNoActivityFile, err)
	}
	if summary.File.URL == "" {
		return "", fmt.Errorf("%w: stored Wahoo workout summary does not name a file", activity.ErrNoActivityFile)
	}

	return summary.File.URL, nil
}

// ActivitySummary reads one activity's totals and Wahoo's own summary document.
// The FIT file the summary names is downloaded separately, by the records phase.
func (p *wahooProvider) ActivitySummary(
	ctx context.Context, accessToken string, id int64,
) (activity.Summary, error) {
	client, err := p.current()
	if err != nil {
		return activity.Summary{}, err
	}
	summary, err := client.WorkoutSummary(ctx, accessToken, id)
	if err != nil {
		return activity.Summary{}, fmt.Errorf("reading a Wahoo workout summary: %w", err)
	}

	return activitySummaryOf(summary), nil
}

// activitySummaryOf maps one Wahoo summary to the totals this service stores.
// Wahoo's active and total durations are this service's moving and elapsed ones.
func activitySummaryOf(summary wahoo.WorkoutSummary) activity.Summary {
	return activity.Summary{
		Raw:            summary.Raw,
		DistanceMetres: summary.DistanceMetres,
		MovingSeconds:  summary.ActiveSeconds,
		ElapsedSeconds: summary.TotalSeconds,
		AscentMetres:   summary.AscentMetres,
	}
}

// IsUnauthorized classifies an error this provider returned. It answers without
// a client, because the sentinel belongs to the package rather than to any one
// client built from it.
func (p *wahooProvider) IsUnauthorized(err error) bool {
	return errors.Is(err, wahoo.ErrUnauthorized)
}

// IsUnreadable reports a summary rejection that belongs to one workout alone.
func (p *wahooProvider) IsUnreadable(err error) bool {
	return errors.Is(err, wahoo.ErrWorkoutUnreadable)
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
	snapshot := settings.Snapshot()
	configured := snapshot.Values().Sources
	built := make([]syncservice.Source, 0, len(configured))
	for _, source := range configured {
		emailName, passwordName, known := runtimeconfig.SourceSecretNames(source.Provider)
		if !known {
			return nil, fmt.Errorf("unknown source provider %q", source.Provider)
		}
		email := snapshot.Secret(emailName).Bytes()
		password := snapshot.Secret(passwordName).Bytes()
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

// sourceFor builds one library's client. Unlike sources it never refuses for
// another library's missing credentials; an unconfigured provider returns no
// client rather than an error.
func sourceFor(
	settings *runtimeconfig.Current, provider route.Provider,
) (source syncservice.Source, configured bool, err error) {
	snapshot := settings.Snapshot()
	for _, entry := range snapshot.Values().Sources {
		if entry.Provider != provider {
			continue
		}
		emailName, passwordName, known := runtimeconfig.SourceSecretNames(entry.Provider)
		if !known {
			return nil, false, fmt.Errorf("unknown source provider %q", entry.Provider)
		}
		email := snapshot.Secret(emailName).Bytes()
		password := snapshot.Secret(passwordName).Bytes()
		if len(email) == 0 || len(password) == 0 {
			return nil, false, nil
		}
		client, err := newSource(entry, email, password)
		if err != nil {
			return nil, false, err
		}

		return client, true, nil
	}

	return nil, false, nil
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

// rideModelProvider predicts moving time from the coefficient pair stored in
// the state database, or from the built-in default until a calibration replaces
// it. Nothing here re-reads the row: whatever stores a new pair calls reload.
type rideModelProvider struct {
	store      *sqlite.Store
	predictor  *ridemodel.Predictor
	validation *httpapi.RideModelValidation
	// fingerprintInForce is what a calibration compares its own fit against, so
	// an unchanged pair is not stored and does not drop cached predictions.
	fingerprintInForce string
	status             httpapi.RideModelStatus
	mutex              sync.Mutex
}

// newRideModelProvider reports the built-in pair until reload has read the row,
// so a startup that fails to read it still shows what prediction would use.
func newRideModelProvider(store *sqlite.Store) *rideModelProvider {
	defaults := ridemodel.Default()

	return &rideModelProvider{
		store:              store,
		fingerprintInForce: defaults.Fingerprint,
		status: httpapi.RideModelStatus{
			SecondsPerKM: defaults.SecondsPerKM, SecondsPerAscentM: defaults.SecondsPerAscentM,
		},
	}
}

// reload reads the stored pair and rebuilds the predictor around it. Predictions
// cached against a different pair are dropped as part of the swap.
func (p *rideModelProvider) reload(ctx context.Context) error {
	coefficients, calibrated, err := p.store.RideModelCoefficients(ctx)
	if err != nil {
		return fmt.Errorf("reading ride model coefficients: %w", err)
	}
	if !calibrated {
		coefficients = ridemodel.Default()
	}
	if validateErr := coefficients.Validate(); validateErr != nil {
		return fmt.Errorf("ride model coefficients: %w", validateErr)
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if err := p.store.PruneStageDurationsWithDifferentFingerprint(ctx, coefficients.Fingerprint); err != nil {
		return fmt.Errorf("pruning stale ride model predictions: %w", err)
	}
	p.predictor = ridemodel.NewPredictor(p.store, coefficients)
	p.validation = nil
	if coefficients.HasValidation() {
		p.validation = &httpapi.RideModelValidation{
			BiasPercent:    coefficients.BiasPercent,
			MAEPercent:     coefficients.MAEPercent,
			P90Percent:     coefficients.P90Percent,
			EvaluatedRides: coefficients.EvaluatedRides,
		}
	}
	p.fingerprintInForce = coefficients.Fingerprint
	p.status = httpapi.RideModelStatus{
		CalibrationCutoff: coefficients.CalibrationCutoff,
		SecondsPerKM:      coefficients.SecondsPerKM,
		SecondsPerAscentM: coefficients.SecondsPerAscentM,
		EvaluatedRides:    coefficients.EvaluatedRides,
		Calibrated:        calibrated,
	}

	return nil
}

func (p *rideModelProvider) current() *ridemodel.Predictor {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return p.predictor
}

// validationView is what the stage endpoint reports about the model's measured
// accuracy, or nothing when the pair in force carries none.
func (p *rideModelProvider) validationView() *httpapi.RideModelValidation {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return p.validation
}

// fingerprint identifies the pair predictions are made with, which is how a
// calibration tells a new fit from the one already in force.
func (p *rideModelProvider) fingerprint() string {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return p.fingerprintInForce
}

// statusView is the coefficient pair the settings page shows.
func (p *rideModelProvider) statusView() httpapi.RideModelStatus {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return p.status
}

// predictorFor hides the provider behind the one method synchronization needs.
// It predicts with the pair the last reload put in force, never re-reading it.
func predictorFor(provider *rideModelProvider) syncservice.Predictor {
	return predictorFunc(func(ctx context.Context, stages []route.Route) (predicted, failed int, err error) {
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
