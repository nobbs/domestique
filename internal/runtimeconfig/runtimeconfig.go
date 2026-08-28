// Package runtimeconfig owns the settings an operator changes while the service
// is running, together with the rules those settings have to satisfy.
//
// The service's remaining configuration is read once from a file at startup and
// is the business of internal/config. What lives here is the half that has no
// reason to cost a restart: a deletion gate flipped for one deliberate run, a
// basemap added to the picker, a notification quieted for a week. Those are
// held in the database, edited from the browser, and read from a live snapshot
// by whoever needs them.
//
// The validation is here rather than at either edge: both the write path and
// the read-back at startup call the same functions, so a value that reaches the
// database is one that would have passed the check a file-loaded value passes.
package runtimeconfig

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync/atomic"
	"time"

	"github.com/nobbs/domestique/internal/route"
)

// Values is one complete set of runtime settings. It is copied by value and
// replaced whole: an edit writes every field, because the form the operator
// submits holds every field.
type Values struct {
	Notifications Notifications
	Wahoo         Wahoo
	RideModel     RideModel
	// Basemaps are the cartographies the reader may switch the map between, in
	// the order they are offered. The first is what a browser that has never
	// chosen one loads. At least one is required, because the map has to paint
	// on something.
	Basemaps []Basemap
	// Sources are the libraries a run reads, in the order it reads them. An
	// empty list is a service that has not been configured yet, not an error.
	Sources []Source
	Surface Surface
	Sync    Sync
}

// Wahoo configures the OAuth application and the destination slots it writes.
type Wahoo struct {
	APIBaseURL   string
	OAuthBaseURL string
	ClientID     string

	// Targets are the destination slot names, in the order they are offered.
	// A slot name is the identity every stored authorization, target stage and
	// recorded run carries, so renaming one abandons that slot's state rather
	// than moving it.
	Targets []string
}

// Source is one library a run reads.
type Source struct {
	Provider route.Provider

	// BaseURL is both the origin the adapter reaches and the one the page links
	// a stage back to, so it is the provider's own web application rather than
	// an API host that happens to answer.
	BaseURL string
}

// RideModel configures the predicted moving time internal/ridemodel computes
// per stage.
type RideModel struct {
	// CoefficientsFile names the coefficient file the offline fitting tooling
	// emits. It is not a secret: it carries fitted physical constants, not a
	// credential or route data.
	//
	// No file switches prediction off entirely, which is also the default.
	CoefficientsFile string
}

// Sync holds the reconciliation settings a run reads when it starts.
type Sync struct {
	// AllowEmptySourceDeletion permits a trusted but empty source to delete the
	// final owned destination routes. It is turned on for one deliberate run and
	// off again afterwards.
	AllowEmptySourceDeletion bool

	// StaleAfter bounds how long the trusted source inventory may go without a
	// successful refresh before it is reported and notified as stale.
	StaleAfter time.Duration

	// InitialDelay is how long after start the first run is attempted. It is
	// read once, by the start it delays, so an edit reaches the next restart
	// rather than the next run.
	InitialDelay time.Duration
}

// Notifications holds what reaches the operator's phone, and where it is sent.
type Notifications struct {
	// Policy names what a routine successful run notifies.
	Policy SuccessPolicy

	// PushoverBaseURL is the origin the application token and user key are sent
	// to. It is a setting so a development or demo environment can point it at
	// an address that goes nowhere rather than reaching the real one with a
	// placeholder token.
	PushoverBaseURL string

	// DigestInterval is how much time separates two digests. It is read only by
	// SuccessPolicyDigest, and validated whatever the policy is: a setting an
	// operator will one day switch to should not turn out to have been invalid
	// all along at the moment they switch.
	DigestInterval time.Duration

	// Enabled is the switch for the whole channel. Off suppresses failures too,
	// not only routine success, which is why every surface that offers it has to
	// say so.
	Enabled bool
}

// SuccessPolicy names what a routine successful run notifies. It never governs
// a failure, a blocked run, or the first success that ends one: those are the
// signals the operator installed notifications for, and only Notifications.
// Enabled silences them.
type SuccessPolicy string

const (
	// SuccessPolicyEvery pushes one message per successful run.
	SuccessPolicyEvery SuccessPolicy = "every"
	// SuccessPolicyQuiet pushes nothing for a routine success.
	SuccessPolicyQuiet SuccessPolicy = "quiet"
	// SuccessPolicyDigest replaces routine success pushes with one aggregate
	// message per interval.
	SuccessPolicyDigest SuccessPolicy = "digest"
)

// Basemap is one cartography the map can load.
type Basemap struct {
	// Name labels the entry in the picker and is the identity a browser
	// remembers its choice by, so it is required and unique across the list.
	Name string

	// StyleURL is the MapLibre style document the operator's browser loads. It
	// is deliberately not a secret: it is served to the page and is visible to
	// anyone who can reach the UI. The default is a keyless provider, so no
	// credential is exposed. A provider that requires an API key would place it
	// in this URL's query and thereby publish it to the browser.
	StyleURL string

	// StyleURLDark is loaded in place of StyleURL when the browser reports a
	// dark system colour scheme. It must share this entry's StyleURL origin. An
	// empty value keeps one style in both schemes.
	StyleURLDark string

	// DarkCartography says this entry's ground is dark whatever the system
	// colour scheme is, which is what satellite imagery is. Anything the page
	// paints over the map reads this rather than the scheme.
	//
	// It contradicts StyleURLDark: a provider publishing a dark twin has light
	// cartography to switch away from. Configuring both is refused.
	DarkCartography bool
}

// Surface configures the local map the road surface of a stage is read from.
type Surface struct {
	// Regions are the OpenStreetMap extracts to index, as Geofabrik slugs such
	// as "europe/germany/rheinland-pfalz". They have to cover the ground the
	// operator actually rides: a stage outside every configured region is served
	// without a surface rather than wrongly.
	//
	// An empty list switches surface classification off entirely, which is also
	// the default: nothing is downloaded, nothing is built, and stages carry no
	// surface.
	Regions []string

	// RebuildInterval is how often the index is rebuilt from freshly published
	// extracts. A rebuild that finds every extract unchanged costs one small
	// request per region and stops there.
	RebuildInterval time.Duration
}

// Store is the durable home of the runtime settings, satisfied by *sqlite.Store.
type Store interface {
	RuntimeSettings(ctx context.Context) (Values, error)
	SetRuntimeSettings(ctx context.Context, values Values) error
	RuntimeSecrets(ctx context.Context) (map[SecretName]Secret, error)
	// SetRuntimeSecrets replaces only the credentials it is given. A secret
	// carrying no bytes is removed rather than stored empty.
	SetRuntimeSecrets(ctx context.Context, secrets map[SecretName]Secret) error
}

// Current holds whichever settings are live and hands them to readers.
//
// Every reader takes a copy for the length of one run, one request, or one
// notification, so an edit lands between two units of work rather than inside
// one.
type Current struct {
	values     atomic.Pointer[Values]
	secrets    atomic.Pointer[map[SecretName]Secret]
	store      Store
	generation atomic.Uint64
}

// Load reads the stored settings and validates them before anything runs on
// them. A database edited by hand into something the write path would have
// refused fails startup here, naming the setting, rather than being served.
func Load(ctx context.Context, store Store) (*Current, error) {
	stored, err := store.RuntimeSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading runtime settings: %w", err)
	}

	validated, err := stored.Validate()
	if err != nil {
		return nil, fmt.Errorf("stored runtime settings: %w", err)
	}

	secrets, err := store.RuntimeSecrets(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading runtime secrets: %w", err)
	}

	current := &Current{store: store}
	current.values.Store(&validated)
	current.secrets.Store(&secrets)

	return current, nil
}

// Generation counts the edits that have landed. Whoever builds something out of
// these settings — a client that has to be constructed, a file that has to be
// read — keeps what it built until this number moves.
func (c *Current) Generation() uint64 {
	return c.generation.Load()
}

// Secret returns one stored credential, or an unset one when it is absent.
func (c *Current) Secret(name SecretName) Secret {
	return (*c.secrets.Load())[name]
}

// SecretIsSet reports whether a credential is stored. It exists so a surface
// that has to say whether one is configured can be given that alone, without
// being given a way to read it.
func (c *Current) SecretIsSet(name SecretName) bool {
	return c.Secret(name).IsSet()
}

// SetSecrets stores the credentials it is given and leaves every other one as
// it was, which is what lets a settings page offer a replacement without ever
// having been told the current value.
func (c *Current) SetSecrets(ctx context.Context, secrets map[SecretName]Secret) error {
	// An edit that replaced no credential is not a write. The settings page sends
	// only the credentials that were typed, so every other save would otherwise
	// open a transaction and move the generation that tells everything holding
	// something built from these settings to look again.
	if len(secrets) == 0 {
		return nil
	}
	for name := range secrets {
		if !slices.Contains(SecretNames(), name) {
			return fmt.Errorf("unknown secret %q", name)
		}
	}
	if err := c.store.SetRuntimeSecrets(ctx, secrets); err != nil {
		return fmt.Errorf("storing runtime secrets: %w", err)
	}

	stored := *c.secrets.Load()
	replaced := make(map[SecretName]Secret, len(stored)+len(secrets))
	maps.Copy(replaced, stored)
	for name, secret := range secrets {
		if secret.IsSet() {
			replaced[name] = secret
		} else {
			delete(replaced, name)
		}
	}
	c.secrets.Store(&replaced)
	c.generation.Add(1)

	return nil
}

// Values returns the live settings. The lists are copied, so a reader holding a
// snapshot cannot be changed underneath by the next edit.
func (c *Current) Values() Values {
	return c.values.Load().clone()
}

// Set validates, persists, and then publishes a complete new set of settings.
// The order is what makes the snapshot and the database agree: a set of values
// that fails validation or fails to be written never becomes live.
//
//nolint:gocritic // value param: a pointer would let the caller mutate what became live after validation.
func (c *Current) Set(ctx context.Context, values Values) error {
	validated, err := values.Validate()
	if err != nil {
		return err
	}
	if err := c.store.SetRuntimeSettings(ctx, validated); err != nil {
		return fmt.Errorf("storing runtime settings: %w", err)
	}
	c.values.Store(&validated)
	c.generation.Add(1)

	return nil
}

// clone copies the slice-valued fields so no two holders of a Values share a
// backing array.
//
//nolint:gocritic // value receiver: cloning a snapshot starts from a copy of it.
func (v Values) clone() Values {
	v.Basemaps = slices.Clone(v.Basemaps)
	v.Sources = slices.Clone(v.Sources)
	v.Surface.Regions = slices.Clone(v.Surface.Regions)
	v.Wahoo.Targets = slices.Clone(v.Wahoo.Targets)

	return v
}

// Missing names every setting still to be entered, in the order a settings page
// offers them. Everything a run needs is here, and so are the Pushover
// credentials while notifications are on, which no run needs but every
// notification does. An empty result is a service that is configured.
func (c *Current) Missing() []string {
	values := c.Values()
	missing := make([]string, 0)
	for _, setting := range []struct{ name, value string }{
		{"wahoo.api_base_url", values.Wahoo.APIBaseURL},
		{"wahoo.oauth_base_url", values.Wahoo.OAuthBaseURL},
		{"wahoo.client_id", values.Wahoo.ClientID},
	} {
		if setting.value == "" {
			missing = append(missing, setting.name)
		}
	}
	if !c.Secret(SecretWahooClientSecret).IsSet() {
		missing = append(missing, string(SecretWahooClientSecret))
	}
	if len(values.Wahoo.Targets) == 0 {
		missing = append(missing, "wahoo.targets")
	}
	if len(values.Sources) == 0 {
		missing = append(missing, "sources")
	}
	for _, source := range values.Sources {
		email, password, _ := SourceSecretNames(source.Provider)
		for _, name := range []SecretName{email, password} {
			if !c.Secret(name).IsSet() {
				missing = append(missing, string(name))
			}
		}
	}
	if values.Notifications.Enabled {
		for _, name := range []SecretName{SecretPushoverApplicationToken, SecretPushoverUserKey} {
			if !c.Secret(name).IsSet() {
				missing = append(missing, string(name))
			}
		}
	}

	return missing
}
