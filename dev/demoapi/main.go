// Command demoapi seeds a demo database and serves the browser UI's API from it.
//
// The shipped binary verifies a Cloudflare Access assertion and a demo cannot
// obtain one, so this stands up an Access team of its own: a signing key
// generated in memory, published to the production verifier through an
// in-process key-set endpoint, and one assertion minted for the UI dev server.
// The real gate runs; only the team signing it is local, and nothing leaves
// this machine.
//
// Development tooling, not part of the shipped binary. See dev/demo.sh.
package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/nobbs/domestique/internal/cfaccess"
	"github.com/nobbs/domestique/internal/config"
	"github.com/nobbs/domestique/internal/demo"
	"github.com/nobbs/domestique/internal/httpapi"
	"github.com/nobbs/domestique/internal/oauth"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/runtimeconfig"
	"github.com/nobbs/domestique/internal/sqlite"
	"github.com/nobbs/domestique/internal/wahoo"
	"github.com/nobbs/domestique/internal/webui"
)

const (
	// assertionLifetime is how long the minted identity stays usable. It is a
	// working session rather than a day, because an expired assertion is the
	// thing the UI has to keep handling correctly.
	assertionLifetime = 8 * time.Hour

	// signingKeyBits matches the key size Access signs with.
	signingKeyBits = 2048

	httpIdleTimeout       = 60 * time.Second
	httpReadHeaderTimeout = 10 * time.Second
	httpReadTimeout       = 15 * time.Second
	httpWriteTimeout      = 75 * time.Second
	shutdownTimeout       = 5 * time.Second
)

func main() {
	assertionFile := flag.String("assertion-file", "",
		"write the minted Access assertion here for the UI dev server to read")
	states := flag.String("states", "current,unauthorized",
		"comma-separated state per configured target slot: current, failed, or unauthorized")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	if err := run(ctx, *assertionFile, *states); err != nil {
		stop()
		fmt.Fprintf(os.Stderr, "demoapi: %v\n", err)
		os.Exit(1)
	}
	stop()
}

func run(ctx context.Context, assertionFile, states string) error {
	settings, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	store, err := sqlite.Open(ctx, settings.State.DatabasePath, settings.State.EncryptionKey())
	if err != nil {
		return fmt.Errorf("opening state: %w", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "demoapi: closing state: %v\n", closeErr)
		}
	}()

	runtimeSettings, err := runtimeconfig.Load(ctx, store)
	if err != nil {
		return fmt.Errorf("loading runtime settings: %w", err)
	}
	if seedErr := seedSettings(ctx, runtimeSettings, settings.HTTP.BrowserOriginURL); seedErr != nil {
		return seedErr
	}

	slots, err := slotsFor(runtimeSettings.Values().Wahoo.Targets, states)
	if err != nil {
		return err
	}
	if seedErr := seed(ctx, store, slots); seedErr != nil {
		return seedErr
	}

	team, err := newTeam(&settings.Access.Cloudflare)
	if err != nil {
		return err
	}
	assertion, err := team.mint()
	if err != nil {
		return err
	}
	if assertionFile != "" {
		if writeErr := os.WriteFile(assertionFile, []byte(assertion), 0o600); writeErr != nil {
			return fmt.Errorf("writing assertion: %w", writeErr)
		}
	}

	handler, err := newHandler(settings, runtimeSettings, store, team, slots)
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:              settings.HTTP.ListenAddress,
		Handler:           handler,
		IdleTimeout:       httpIdleTimeout,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		ReadTimeout:       httpReadTimeout,
		WriteTimeout:      httpWriteTimeout,
	}

	fmt.Printf("Demo API listening on %s as %s\n",
		settings.HTTP.ListenAddress, settings.Access.Cloudflare.AllowedEmail)

	errs := make(chan error, 1)
	go func() {
		if serveErr := server.ListenAndServe(); serveErr != nil &&
			!errors.Is(serveErr, http.ErrServerClosed) {
			errs <- serveErr

			return
		}
		errs <- nil
	}()

	select {
	case serveErr := <-errs:
		return serveErr
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if shutdownErr := server.Shutdown(shutdownCtx); shutdownErr != nil {
		return fmt.Errorf("shutting down: %w", shutdownErr)
	}

	return nil
}

// newHandler wires the served surface the same way the binary does, minus every
// part a demo must not have: no scheduler, no reporter, and no source or
// destination client behind the sync routes.
func newHandler(
	settings *config.Settings,
	runtimeSettings *runtimeconfig.Current,
	store *sqlite.Store,
	team *team,
	slots []demo.Slot,
) (http.Handler, error) {
	// Built once from the seeded settings rather than per request: a demo has
	// nobody editing them, and nothing here ever reaches the address anyway.
	values := runtimeSettings.Values()
	destination, err := wahoo.New(&wahoo.Options{
		APIBaseURL:   values.Wahoo.APIBaseURL,
		OAuthBaseURL: values.Wahoo.OAuthBaseURL,
		ClientID:     values.Wahoo.ClientID,
		RedirectURL:  settings.HTTP.BrowserOriginURL + "/oauth/wahoo/callback",
		ClientSecret: runtimeSettings.Secret(runtimeconfig.SecretWahooClientSecret).Bytes(),
	})
	if err != nil {
		return nil, fmt.Errorf("creating Wahoo client: %w", err)
	}
	oauthService, err := oauth.New(store, destination)
	if err != nil {
		return nil, fmt.Errorf("creating oauth service: %w", err)
	}

	// A demo has nothing to synchronise from and nowhere to write to, so a manual
	// run re-seeds the library at the current instant instead, on the request
	// rather than in the background. False means one run is already in progress.
	// The full and single-target triggers share one reseeder and one `running`
	// flag; two would let their writes interleave over the same rows.
	demoReseeder := reseeder{
		store:   store,
		slots:   slots,
		running: &atomic.Bool{},
	}
	handler, err := httpapi.New(
		&httpapi.Options{
			Settings:         runtimeSettings,
			Alerts:           newDemoAlerts(),
			BuildRevision:    "demo",
			AccessVerifier:   team,
			AccessEmail:      settings.Access.Cloudflare.AllowedEmail,
			BrowserOriginURL: settings.HTTP.BrowserOriginURL,
		},
		oauthService,
		store,
		httpapi.SyncFuncs{
			TriggerFunc:       demoReseeder.trigger,
			TriggerTargetFunc: demoReseeder.triggerTarget,
			TriggerClearFunc:  demoReseeder.triggerClear,
		},
		bundleAssets(),
		httpapi.WeatherFunc(syntheticWeather),
	)
	if err != nil {
		return nil, fmt.Errorf("creating HTTP handler: %w", err)
	}

	return handler, nil
}

// bundleAssets is the embedded browser UI when one has been built into this
// binary, and an explanation when one has not. It is served here as well as
// through the Vite dev server because this is what a deployment runs: the
// production bundle behind this handler's gate, headers and policy. The bundle is
// embedded at compile time; `dev/demo.sh --with-bundle` guarantees a fresh one.
func bundleAssets() httpapi.Assets {
	assets, err := webui.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "demoapi: no browser UI bundle is embedded (%v); "+
			"the demo serves the UI from the Vite dev server\n", err)

		return unbuiltAssets{}
	}
	fmt.Println("Serving the embedded browser UI bundle")

	return assets
}

// unbuiltAssets stands in for a bundle that was never built. Saying so is more
// useful than serving a blank page.
type unbuiltAssets struct{}

func (unbuiltAssets) Index(writer http.ResponseWriter, _ *http.Request) {
	http.Error(writer, unbuiltMessage, http.StatusNotFound)
}

func (unbuiltAssets) Static(writer http.ResponseWriter, _ *http.Request) {
	http.Error(writer, unbuiltMessage, http.StatusNotFound)
}

const unbuiltMessage = "no browser UI bundle is embedded in this binary: " +
	"run `mise run ui-build` before building it, or use the Vite dev server"

// reseeder runs one demo synchronisation at a time. Two concurrent seeds would
// interleave their writes over the same rows, and the second caller is in the
// state the served surface already has a word for: a run is in progress.
type reseeder struct {
	store   *sqlite.Store
	running *atomic.Bool
	slots   []demo.Slot
}

func (r reseeder) trigger(_ httpapi.SyncPhase) bool {
	if !r.running.CompareAndSwap(false, true) {
		return false
	}
	defer r.running.Store(false)

	if err := seed(context.Background(), r.store, r.slots); err != nil {
		fmt.Fprintf(os.Stderr, "demoapi: re-seeding: %v\n", err)
	}

	return true
}

// triggerTarget re-seeds the whole demo library, the same as trigger: a demo
// target names no real Wahoo account to isolate work against.
func (r reseeder) triggerTarget(_ string) bool {
	return r.trigger("")
}

// triggerClear answers the clear control so the demo can exercise it, and
// re-seeds like every other trigger here. It deletes nothing: a demo target names
// no Wahoo account. What it offers is the control's real shape — accepted,
// refused while something else runs, and the page settling afterwards.
func (r reseeder) triggerClear(_ string) bool {
	return r.trigger("")
}

// seed fills the database with the synthetic library. The clock is the wall
// clock, because a demo whose last run is dated years ago reads as a broken
// service rather than as a fixture.
func seed(ctx context.Context, store *sqlite.Store, slots []demo.Slot) error {
	if err := demo.Seed(ctx, store, slots, time.Now().UTC()); err != nil {
		return fmt.Errorf("seeding the demo library: %w", err)
	}

	return nil
}

// slotsFor pairs each configured target with the state it was asked for. The
// slots come from the settings rather than from the flag alone, so a seeded
// state cannot address a target the served surface does not know about.
func slotsFor(targets []string, states string) ([]demo.Slot, error) {
	requested := strings.Split(states, ",")
	if len(requested) != len(targets) {
		return nil, fmt.Errorf("the demo has %d target slots but %d states were given",
			len(targets), len(requested))
	}

	slots := make([]demo.Slot, 0, len(targets))
	for index, target := range targets {
		state, err := slotState(strings.TrimSpace(requested[index]))
		if err != nil {
			return nil, err
		}
		slots = append(slots, demo.Slot{ID: target, State: state})
	}

	return slots, nil
}

func slotState(name string) (demo.SlotState, error) {
	switch demo.SlotState(name) {
	case demo.SlotCurrent:
		return demo.SlotCurrent, nil
	case demo.SlotFailed:
		return demo.SlotFailed, nil
	case demo.SlotUnauthorized:
		return demo.SlotUnauthorized, nil
	default:
		return "", fmt.Errorf("unknown slot state %q: want current, failed, or unauthorized", name)
	}
}

// team is a local Cloudflare Access team: a signing key, the key-set document
// that publishes it, and the production verifier reading that document.
type team struct {
	verifier *cfaccess.Verifier
	private  *rsa.PrivateKey
	issuer   string
	audience string
	email    string
	keyID    string
}

func newTeam(access *config.CloudflareAccess) (*team, error) {
	private, err := rsa.GenerateKey(rand.Reader, signingKeyBits)
	if err != nil {
		return nil, fmt.Errorf("generating a demo signing key: %w", err)
	}

	local := &team{
		private:  private,
		issuer:   issuerFor(access.TeamDomain),
		audience: access.ApplicationAUD,
		email:    access.AllowedEmail,
		keyID:    "demo",
	}
	// The verifier fetches its key set over HTTPS from the team domain. This
	// transport answers that one request from memory, so the demo publishes its
	// key without a listener, a certificate, or a name to resolve — and cannot
	// reach a real team even by accident.
	verifier, err := cfaccess.New(&cfaccess.Options{
		TeamDomain: access.TeamDomain,
		Audience:   access.ApplicationAUD,
		HTTPClient: &http.Client{Transport: local},
	})
	if err != nil {
		return nil, fmt.Errorf("configuring the demo access team: %w", err)
	}
	local.verifier = verifier

	return local, nil
}

// issuerFor restates how the verifier derives the issuer from a team domain, so
// a minted assertion claims the issuer the verifier expects.
func issuerFor(teamDomain string) string {
	team := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(teamDomain), "https://"), "/")
	if !strings.Contains(team, ".") {
		team += ".cloudflareaccess.com"
	}

	return "https://" + team
}

// RoundTrip answers the verifier's key-set request and refuses everything else.
func (t *team) RoundTrip(request *http.Request) (*http.Response, error) {
	if !strings.HasPrefix(request.URL.String(), t.issuer+"/") {
		return nil, fmt.Errorf("demo access team was asked for %s", request.URL.Host)
	}

	document, err := json.Marshal(map[string]any{
		"keys": []map[string]string{{
			"kid": t.keyID,
			"kty": "RSA",
			"alg": "RS256",
			"use": "sig",
			"n":   base64.RawURLEncoding.EncodeToString(t.private.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(t.private.E)).Bytes()),
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("rendering the demo key set: %w", err)
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(document))),
		Request:    request,
	}, nil
}

// Verify is the production gate, reading the local team's key set.
func (t *team) Verify(ctx context.Context, assertion string) (string, error) {
	identity, err := t.verifier.Verify(ctx, assertion)
	if err != nil {
		return "", fmt.Errorf("verifying demo assertion: %w", err)
	}

	return identity.Email, nil
}

// mint signs one assertion for the configured identity.
func (t *team) mint() (string, error) {
	now := time.Now()
	header := map[string]any{"alg": "RS256", "kid": t.keyID, "typ": "JWT"}
	claims := map[string]any{
		"iss":   t.issuer,
		"aud":   []string{t.audience},
		"email": t.email,
		"sub":   "demo-subject",
		"iat":   now.Unix(),
		"nbf":   now.Unix(),
		"exp":   now.Add(assertionLifetime).Unix(),
	}

	segment := func(value map[string]any) (string, error) {
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("encoding assertion: %w", err)
		}

		return base64.RawURLEncoding.EncodeToString(encoded), nil
	}
	encodedHeader, err := segment(header)
	if err != nil {
		return "", err
	}
	encodedClaims, err := segment(claims)
	if err != nil {
		return "", err
	}

	signingInput := encodedHeader + "." + encodedClaims
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, t.private, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("signing assertion: %w", err)
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// demoYear is how far out the demo pushes its first run: far enough that the
// schedule never fires, so the only synchronisation is one somebody asked for.
const demoYear = 365 * 24 * time.Hour

// seedSettings writes everything the demo is configured with, which is everything
// but its listener, its identity gate and its state file. The cartographies are
// the reason it exists: the picker appears only with more than one entry, and
// these are one provider's distinct styles. Every address is the demo's own
// origin and every credential a placeholder. Written on every start, since the
// settings page can edit them.
func seedSettings(ctx context.Context, current *runtimeconfig.Current, origin string) error {
	secrets := make(map[runtimeconfig.SecretName]runtimeconfig.Secret, len(runtimeconfig.SecretNames()))
	for _, name := range runtimeconfig.SecretNames() {
		secrets[name] = runtimeconfig.NewSecret([]byte("demo-placeholder"))
	}
	if err := current.SetSecrets(ctx, secrets); err != nil {
		return fmt.Errorf("seeding the demo credentials: %w", err)
	}

	values := current.Values()
	values.Wahoo = runtimeconfig.Wahoo{
		APIBaseURL:   origin,
		OAuthBaseURL: origin,
		ClientID:     "demo-placeholder",
		Targets:      []string{"rider-a", "rider-b"},
	}
	values.Sources = []runtimeconfig.Source{
		{Provider: route.ProviderVeloPlanner, BaseURL: origin},
	}
	values.Sync.InitialDelay = demoYear
	values.Basemaps = []runtimeconfig.Basemap{
		{
			Name:         "Streets",
			StyleURL:     "https://tiles.openfreemap.org/styles/bright",
			StyleURLDark: "https://tiles.openfreemap.org/styles/dark",
		},
		{Name: "Positron", StyleURL: "https://tiles.openfreemap.org/styles/positron"},
		{Name: "Liberty", StyleURL: "https://tiles.openfreemap.org/styles/liberty"},
		{
			Name:            "Dark",
			StyleURL:        "https://tiles.openfreemap.org/styles/dark",
			DarkCartography: true,
		},
		{Name: "Fiord", StyleURL: "https://tiles.openfreemap.org/styles/fiord"},
	}
	if err := current.Update(ctx, func(runtimeconfig.Values) runtimeconfig.Values { return values }); err != nil {
		return fmt.Errorf("seeding the demo settings: %w", err)
	}

	return nil
}
