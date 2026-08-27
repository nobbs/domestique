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
// The validation is here rather than at either edge on purpose. Both the write
// path and the read-back at startup call the same functions, so a value that
// reaches the database is one that would have passed the check a file-loaded
// value passes. Two copies of these rules, drifting apart, is the failure this
// package is shaped to prevent.
package runtimeconfig

import (
	"context"
	"fmt"
	"slices"
	"sync/atomic"
	"time"
)

// Values is one complete set of runtime settings. It is copied by value and
// replaced whole: an edit writes every field, because the form the operator
// submits holds every field.
type Values struct {
	Sync          Sync
	Notifications Notifications
	// Basemaps are the cartographies the reader may switch the map between, in
	// the order they are offered. The first is what a browser that has never
	// chosen one loads. At least one is required, because the map has to paint
	// on something.
	Basemaps []Basemap
	Surface  Surface
}

// Sync holds the reconciliation settings a run reads when it starts.
type Sync struct {
	// AllowEmptySourceDeletion permits a trusted but empty source to delete the
	// final owned destination routes. It is the switch an operator turns on for
	// one deliberate run and off again afterwards, which is the whole reason it
	// stopped being a file key.
	AllowEmptySourceDeletion bool

	// StaleAfter bounds how long the trusted source inventory may go without a
	// successful refresh before it is reported and notified as stale.
	StaleAfter time.Duration
}

// Notifications holds what reaches the operator's phone, and where it is sent.
type Notifications struct {
	// Enabled is the switch for the whole channel. Off suppresses failures too,
	// not only routine success, which is why every surface that offers it has to
	// say so.
	Enabled bool

	// Policy names what a routine successful run notifies.
	Policy SuccessPolicy

	// DigestInterval is how much time separates two digests. It is read only by
	// SuccessPolicyDigest, and validated whatever the policy is: a setting an
	// operator will one day switch to should not turn out to have been invalid
	// all along at the moment they switch.
	DigestInterval time.Duration

	// PushoverBaseURL is the origin the application token and user key are sent
	// to. It is configurable because a development or demo environment has to be
	// able to point it at an address that goes nowhere, and the alternative is a
	// compiled-in host such an environment would quietly reach with a
	// placeholder token.
	PushoverBaseURL string
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
	// paints over the map reads this rather than the scheme, because a route
	// drawn in the dark-ground ink over light cartography — or the reverse —
	// is the one that cannot be seen.
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
	// An empty list switches surface classification off entirely. Nothing is
	// downloaded, nothing is built, and stages simply carry no surface — which
	// is also what a deployment gets by default, because the right regions are
	// a property of where somebody rides and cannot be guessed.
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
}

// Current holds whichever settings are live and hands them to readers.
//
// Every reader takes a copy for the length of one run, one request, or one
// notification, so an edit lands between two units of work rather than inside
// one. It mirrors osmindex.Current, the holder this service already uses for
// state that is replaced under running readers.
type Current struct {
	values atomic.Pointer[Values]
	store  Store
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

	current := &Current{store: store}
	current.values.Store(&validated)

	return current, nil
}

// Values returns the live settings. The lists are copied, so a reader holding a
// snapshot cannot be changed underneath by the next edit.
func (c *Current) Values() Values {
	return c.values.Load().clone()
}

// Set validates, persists, and then publishes a complete new set of settings.
// The order is what makes the snapshot and the database agree: a set of values
// that fails validation or fails to be written never becomes live.
func (c *Current) Set(ctx context.Context, values Values) error {
	validated, err := values.Validate()
	if err != nil {
		return err
	}
	if err := c.store.SetRuntimeSettings(ctx, validated); err != nil {
		return fmt.Errorf("storing runtime settings: %w", err)
	}
	c.values.Store(&validated)

	return nil
}

// clone copies the slice-valued fields so no two holders of a Values share a
// backing array.
func (v Values) clone() Values {
	v.Basemaps = slices.Clone(v.Basemaps)
	v.Surface.Regions = slices.Clone(v.Surface.Regions)

	return v
}
