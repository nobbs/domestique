// Package runtimeconfig owns the settings an operator changes while the service
// is running, and the rules both the write path and startup validate them with.
package runtimeconfig

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nobbs/domestique/internal/route"
)

// Values is one complete set of runtime settings. It is copied by value and
// replaced whole: an edit writes every field.
type Values struct {
	Notifications Notifications
	Wahoo         Wahoo
	RideModel     RideModel
	// Basemaps are the cartographies the reader may switch between, in the order
	// offered. The first is the default; at least one is required.
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

	// Targets are the destination slot names, in order. Stored authorizations and
	// runs carry the name, so renaming a slot abandons its state.
	Targets []string
}

// SourceProviders lists the libraries a run can read, in the order it reads them
// and a settings page offers them.
func SourceProviders() []route.Provider {
	return []route.Provider{route.ProviderVeloPlanner, route.ProviderKomoot}
}

// Source is one library a run reads.
type Source struct {
	Provider route.Provider

	// BaseURL is both the origin the adapter reaches and the one the page links a
	// stage back to: the provider's web application, not an API host.
	BaseURL string
}

// RideModel configures the predicted moving time internal/ridemodel computes
// per stage.
type RideModel struct {
	// CoefficientsFile names the file the offline fitting tooling emits. Not a
	// secret. Empty switches prediction off entirely, and is the default.
	CoefficientsFile string
}

// Sync holds the reconciliation settings a run reads when it starts.
type Sync struct {
	// AllowEmptySourceDeletion permits a trusted but empty source to delete the
	// final owned destination routes.
	AllowEmptySourceDeletion bool

	// StaleAfter bounds how long the trusted source inventory may go without a
	// successful refresh before it is reported and notified as stale.
	StaleAfter time.Duration

	// InitialDelay is how long after start the first run is attempted. Read once,
	// by the start it delays, so an edit reaches the next restart.
	InitialDelay time.Duration
}

// Notifications holds what reaches the operator's phone, and where it is sent.
// Which of them are sent is not here: that is a decision per task and per
// reason, and it lives in the alert matrix.
type Notifications struct {
	// PushoverBaseURL is the origin the application token and user key are sent to.
	// A setting so a demo environment can point it somewhere that goes nowhere.
	PushoverBaseURL string

	// Enabled is the switch for the whole channel. Off suppresses failures too,
	// not only routine success.
	Enabled bool
}

// Basemap is one cartography the map can load.
type Basemap struct {
	// Name labels the entry in the picker and is the identity a browser
	// remembers its choice by, so it is required and unique across the list.
	Name string

	// StyleURL is the MapLibre style the browser loads. Not a secret: it is served
	// to the page, so a provider's key in its query is published to the browser.
	StyleURL string

	// StyleURLDark replaces StyleURL under a dark system colour scheme. It must
	// share this entry's StyleURL origin; empty keeps one style in both schemes.
	StyleURLDark string

	// DarkCartography says this entry's ground is dark whatever the colour scheme,
	// as satellite imagery is. It contradicts StyleURLDark; setting both is refused.
	DarkCartography bool
}

// Surface configures the local map the road surface of a stage is read from.
type Surface struct {
	// Regions are the OpenStreetMap extracts to index, as Geofabrik slugs. A stage
	// outside every region is served without a surface. Empty disables the feature.
	Regions []string

	// RebuildInterval is how often the index is rebuilt. A rebuild that finds every
	// extract unchanged costs one small request per region.
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

// Current holds whichever settings are live and hands them to readers. Every
// reader takes a copy for one run, request, or notification.
type Current struct {
	values  atomic.Pointer[Values]
	secrets atomic.Pointer[map[SecretName]Secret]
	store   Store
	// writing serialises the read-modify-write that every section edit performs,
	// over its settings and its credentials alike.
	writing    sync.Mutex
	generation atomic.Uint64
}

// ErrStore reports that the store refused a write, separating a service that
// cannot answer now from a value the rules will never accept.
var ErrStore = errors.New("the settings could not be stored")

// Load reads the stored settings and validates them before anything runs on
// them. A hand-edited database fails startup here, naming the setting.
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

// Generation counts the edits that have landed. A caller holding something built
// from these settings keeps it until this number moves.
func (c *Current) Generation() uint64 {
	return c.generation.Load()
}

// Secret returns one stored credential, or an unset one when it is absent.
func (c *Current) Secret(name SecretName) Secret {
	return (*c.secrets.Load())[name]
}

// SecretIsSet reports whether a credential is stored, without providing any way
// to read it.
func (c *Current) SecretIsSet(name SecretName) bool {
	return c.Secret(name).IsSet()
}

// SetSecrets stores the credentials it is given and leaves every other one as it
// was, so a page can offer a replacement without knowing the current value.
func (c *Current) SetSecrets(ctx context.Context, secrets map[SecretName]Secret) error {
	// An edit that replaced no credential is not a write: it would otherwise move
	// the generation and make every holder rebuild.
	if len(secrets) == 0 {
		return nil
	}
	// Read-modify-write under the same lock as Update: two sections saved at once
	// each carry their own credentials, and the later write would drop the earlier.
	c.writing.Lock()
	defer c.writing.Unlock()

	for name := range secrets {
		if !slices.Contains(SecretNames(), name) {
			return fmt.Errorf("unknown secret %q", name)
		}
	}
	if err := c.store.SetRuntimeSecrets(ctx, secrets); err != nil {
		return fmt.Errorf("%w: %w", ErrStore, err)
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

// Update validates, persists, then publishes; values that fail either never
// become live. Read-change-write is one critical section.
func (c *Current) Update(ctx context.Context, change func(Values) Values) error {
	c.writing.Lock()
	defer c.writing.Unlock()

	validated, err := change(c.values.Load().clone()).Validate()
	if err != nil {
		return err
	}
	if err := c.store.SetRuntimeSettings(ctx, validated); err != nil {
		return fmt.Errorf("%w: %w", ErrStore, err)
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
// offers them, including the Pushover credentials while notifications are on.
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
