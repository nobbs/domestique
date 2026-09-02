// Command demoapi seeds a demo database and serves the browser UI's API from it.
//
// The shipped binary admits a browser session and a demo has no tenant to sign
// in against, so this stands up an Auth0-shaped issuer of its own on a loopback
// port, and pre-mints one session so the UI dev server can load without a
// round trip through it. The real gate runs; only the tenant behind it is
// local, and nothing leaves this machine.
//
// Development tooling, not part of the shipped binary. See dev/demo.sh.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/nobbs/domestique/internal/config"
	"github.com/nobbs/domestique/internal/demo"
	"github.com/nobbs/domestique/internal/httpapi"
	"github.com/nobbs/domestique/internal/oauth"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/runtimeconfig"
	"github.com/nobbs/domestique/internal/session"
	"github.com/nobbs/domestique/internal/sqlite"
	"github.com/nobbs/domestique/internal/wahoo"
	"github.com/nobbs/domestique/internal/webui"
)

const (
	// sessionLifetime matches internal/session's own, so the pre-minted row
	// expires the way one created by a real sign-in does.
	sessionLifetime = 30 * 24 * time.Hour

	httpIdleTimeout       = 60 * time.Second
	httpReadHeaderTimeout = 10 * time.Second
	httpReadTimeout       = 15 * time.Second
	httpWriteTimeout      = 75 * time.Second
	shutdownTimeout       = 5 * time.Second
)

func main() {
	sessionFile := flag.String("session-file", "",
		"write the pre-minted session token here for the UI dev server to read")
	states := flag.String("states", "current,unauthorized",
		"comma-separated state per configured target slot: current, failed, or unauthorized")
	callbackURL := flag.String("callback-url", "",
		"browser-reachable URL for /auth/callback; defaults to http://127.0.0.1:<listen port>/auth/callback")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	if err := run(ctx, *sessionFile, *states, *callbackURL); err != nil {
		stop()
		fmt.Fprintf(os.Stderr, "demoapi: %v\n", err)
		os.Exit(1)
	}
	stop()
}

func run(ctx context.Context, sessionFile, states, callbackURL string) error {
	settings, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	if callbackURL == "" {
		callbackURL, err = defaultCallbackURL(settings.HTTP.ListenAddress)
		if err != nil {
			return err
		}
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

	tenant, err := newIssuer(settings.Auth.Auth0.Domain, settings.Auth.Auth0.ClientID, callbackURL)
	if err != nil {
		return err
	}
	stopIssuer, err := tenant.serve()
	if err != nil {
		return err
	}
	defer stopIssuer()

	provider, err := tenant.client(settings.Auth.Auth0.ClientSecret(),
		settings.HTTP.BrowserOriginURL+"/auth/callback")
	if err != nil {
		return err
	}
	sessions, err := session.New(store, signInProvider{client: provider},
		settings.Auth.Auth0.AllowedSubjects, nil)
	if err != nil {
		return fmt.Errorf("creating the session service: %w", err)
	}
	// Pre-minted so the dev server and the browser suite can load a gated page
	// without walking the flow through a self-signed issuer first.
	if mintErr := mintSession(ctx, store, sessionFile); mintErr != nil {
		return mintErr
	}

	handler, err := newHandler(settings, runtimeSettings, store, sessions, slots)
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

	fmt.Printf("Demo API listening on %s as %s\n", settings.HTTP.ListenAddress, demoSubject)

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

// defaultCallbackURL is where the demo issuer sends the browser back to when no
// -callback-url was given: the demo API's own loopback port, since the
// configured listen address's host is typically empty (every interface).
func defaultCallbackURL(listenAddress string) (string, error) {
	_, port, err := net.SplitHostPort(listenAddress)
	if err != nil {
		return "", fmt.Errorf("reading the demo listen address: %w", err)
	}

	return fmt.Sprintf("http://127.0.0.1:%s/auth/callback", port), nil
}

// newHandler wires the served surface the same way the binary does, minus every
// part a demo must not have: no scheduler, no reporter, and no source or
// destination client behind the sync routes.
func newHandler(
	settings *config.Settings,
	runtimeSettings *runtimeconfig.Current,
	store *sqlite.Store,
	sessions httpapi.Sessions,
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
			Tasks:            newDemoTasks(demoReseeder.trigger),
			BuildRevision:    "demo",
			Sessions:         sessions,
			BrowserOriginURL: settings.HTTP.BrowserOriginURL,
		},
		oauthService,
		store,
		httpapi.SyncFuncs{},
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

func (r reseeder) trigger() bool {
	if !r.running.CompareAndSwap(false, true) {
		return false
	}
	defer r.running.Store(false)

	if err := seed(context.Background(), r.store, r.slots); err != nil {
		fmt.Fprintf(os.Stderr, "demoapi: re-seeding: %v\n", err)
	}

	return true
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

// mintSession writes one browser session straight into the state database and
// leaves the raw token where the dev server can read it.
func mintSession(ctx context.Context, store *sqlite.Store, path string) error {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Errorf("reading randomness: %w", err)
	}
	digest := sha256.Sum256(raw)
	now := time.Now().UTC()
	if err := store.CreateSession(ctx, digest[:], demoSubject, demoDisplay, now, now.Add(sessionLifetime)); err != nil {
		return fmt.Errorf("storing the demo session: %w", err)
	}
	if path == "" {
		return nil
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		return fmt.Errorf("writing the demo session: %w", err)
	}

	return nil
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
