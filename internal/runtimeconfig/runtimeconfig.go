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
	// Timezone is the IANA zone this service reads local time in. One zone
	// rather than one per reader: a run happens once, at a time somebody chose.
	Timezone string

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

// Wahoo configures the shared OAuth application every self-service target
// authorizes against. Which targets exist is not a setting: each is created
// by its own owning subject, on their own first connection.
type Wahoo struct {
	APIBaseURL   string
	OAuthBaseURL string
	ClientID     string
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
	SetRuntimeSettingsAndSecrets(ctx context.Context, values Values, secrets map[SecretName]Secret) error
	RuntimeSecrets(ctx context.Context) (map[SecretName]Secret, error)
	// SetRuntimeSecrets replaces only the credentials it is given. A secret
	// carrying no bytes is removed rather than stored empty.
	SetRuntimeSecrets(ctx context.Context, secrets map[SecretName]Secret) error
}

// Snapshot is one immutable generation of the settings and credentials in force.
type Snapshot struct {
	secrets    map[SecretName]Secret
	values     Values
	generation uint64
}

// Values returns this snapshot's settings with independent list storage.
func (s *Snapshot) Values() Values {
	return s.values.clone()
}

// Secret returns one credential from this snapshot, or an unset one when absent.
func (s *Snapshot) Secret(name SecretName) Secret {
	return s.secrets[name]
}

// Generation identifies the edit that produced this snapshot.
func (s *Snapshot) Generation() uint64 {
	return s.generation
}

// Current holds whichever settings are live and hands them to readers. Every
// reader takes a copy for one run, request, or notification.
type Current struct {
	snapshot atomic.Pointer[Snapshot]
	store    Store
	// writing serialises the read-modify-write that every section edit performs,
	// over its settings and its credentials alike.
	writing sync.Mutex
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
	current.snapshot.Store(&Snapshot{values: validated, secrets: maps.Clone(secrets)})

	return current, nil
}

// Generation counts the edits that have landed. A caller holding something built
// from these settings keeps it until this number moves.
func (c *Current) Generation() uint64 {
	return c.Snapshot().Generation()
}

// Secret returns one stored credential, or an unset one when it is absent.
func (c *Current) Secret(name SecretName) Secret {
	return c.Snapshot().Secret(name)
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

	if err := validateSecretNames(secrets); err != nil {
		return err
	}
	if err := c.store.SetRuntimeSecrets(ctx, secrets); err != nil {
		return fmt.Errorf("%w: %w", ErrStore, err)
	}

	current := c.snapshot.Load()
	c.snapshot.Store(&Snapshot{
		values: current.values, secrets: mergeSecrets(current.secrets, secrets), generation: current.generation + 1,
	})

	return nil
}

func validateSecretNames(secrets map[SecretName]Secret) error {
	for name := range secrets {
		if !slices.Contains(SecretNames(), name) {
			return fmt.Errorf("unknown secret %q", name)
		}
	}
	return nil
}

// mergeSecrets overlays incoming on current; an unset incoming secret removes the credential.
func mergeSecrets(current, incoming map[SecretName]Secret) map[SecretName]Secret {
	replaced := make(map[SecretName]Secret, len(current)+len(incoming))
	maps.Copy(replaced, current)
	for name, secret := range incoming {
		if secret.IsSet() {
			replaced[name] = secret
		} else {
			delete(replaced, name)
		}
	}
	return replaced
}

// Values returns the live settings. The lists are copied, so a reader holding a
// snapshot cannot be changed underneath by the next edit.
func (c *Current) Values() Values {
	return c.Snapshot().Values()
}

// Snapshot returns one coherent generation for readers that need settings and credentials.
func (c *Current) Snapshot() *Snapshot {
	return c.snapshot.Load()
}

// Update validates, persists, then publishes; values that fail either never
// become live. Read-change-write is one critical section.
func (c *Current) Update(ctx context.Context, change func(Values) Values) error {
	return c.UpdateWithSecrets(ctx, change, nil)
}

// UpdateWithSecrets applies one settings-section edit and its submitted credentials atomically.
func (c *Current) UpdateWithSecrets(ctx context.Context, change func(Values) Values, secrets map[SecretName]Secret) error {
	c.writing.Lock()
	defer c.writing.Unlock()

	current := c.snapshot.Load()
	validated, validateErr := change(current.values.clone()).Validate()
	if validateErr != nil {
		return validateErr
	}
	if err := validateSecretNames(secrets); err != nil {
		return err
	}
	var writeErr error
	if len(secrets) == 0 {
		writeErr = c.store.SetRuntimeSettings(ctx, validated)
	} else {
		writeErr = c.store.SetRuntimeSettingsAndSecrets(ctx, validated, secrets)
	}
	if writeErr != nil {
		return fmt.Errorf("%w: %w", ErrStore, writeErr)
	}
	c.snapshot.Store(&Snapshot{
		values: validated, secrets: mergeSecrets(current.secrets, secrets), generation: current.generation + 1,
	})

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

	return v
}

// Missing names every setting still to be entered, in the order a settings page
// offers them, including the Pushover credentials while notifications are on.
func (c *Current) Missing() []string {
	snapshot := c.Snapshot()
	values := snapshot.Values()
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
	if !snapshot.Secret(SecretWahooClientSecret).IsSet() {
		missing = append(missing, string(SecretWahooClientSecret))
	}
	if len(values.Sources) == 0 {
		missing = append(missing, "sources")
	}
	for _, source := range values.Sources {
		email, password, _ := SourceSecretNames(source.Provider)
		for _, name := range []SecretName{email, password} {
			if !snapshot.Secret(name).IsSet() {
				missing = append(missing, string(name))
			}
		}
	}
	if values.Notifications.Enabled {
		for _, name := range []SecretName{SecretPushoverApplicationToken, SecretPushoverUserKey} {
			if !snapshot.Secret(name).IsSet() {
				missing = append(missing, string(name))
			}
		}
	}

	return missing
}
