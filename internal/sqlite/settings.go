package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/runtimeconfig"
	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
)

// RuntimeSettings reads the settings an operator edits while the service is
// running. It satisfies runtimeconfig.Store. Both lists come back in the order
// they were arranged in: the first basemap is what a browser loads by default.
func (s *Store) RuntimeSettings(ctx context.Context) (runtimeconfig.Values, error) {
	// success_policy and digest_interval_seconds are unused, superseded by the
	// per-task alert matrix; both are NOT NULL with CHECK, so dropping them needs a rebuild.
	row, err := s.queries.GetRuntimeSettings(ctx)
	if err != nil {
		return runtimeconfig.Values{}, fmt.Errorf("reading the runtime settings: %w", err)
	}
	values := runtimeconfig.Values{}
	values.Sync.AllowEmptySourceDeletion = row.AllowEmptySourceDeletion != 0
	values.Sync.StaleAfter = time.Duration(row.StaleAfterSeconds) * time.Second
	values.Sync.InitialDelay = time.Duration(row.SyncInitialDelaySeconds) * time.Second
	values.Notifications.Enabled = row.NotificationsEnabled != 0
	values.Notifications.PushoverBaseURL = row.PushoverBaseUrl
	values.Surface.RebuildInterval = time.Duration(row.SurfaceRebuildIntervalSeconds) * time.Second
	values.Wahoo.APIBaseURL = row.WahooApiBaseUrl
	values.Wahoo.OAuthBaseURL = row.WahooOauthBaseUrl
	values.Wahoo.ClientID = row.WahooClientID
	values.RideModel.CoefficientsFile = row.RidemodelCoefficientsFile
	values.Timezone = row.Timezone

	basemaps, err := s.runtimeBasemaps(ctx)
	if err != nil {
		return runtimeconfig.Values{}, err
	}
	regions, err := s.runtimeSurfaceRegions(ctx)
	if err != nil {
		return runtimeconfig.Values{}, err
	}
	targets, err := s.runtimeTargets(ctx)
	if err != nil {
		return runtimeconfig.Values{}, err
	}
	sources, err := s.runtimeSources(ctx)
	if err != nil {
		return runtimeconfig.Values{}, err
	}
	values.Basemaps = basemaps
	values.Surface.Regions = regions
	values.Wahoo.Targets = targets
	values.Sources = sources

	return values, nil
}

// SetRuntimeSettings replaces every runtime setting in one transaction. The lists
// are deleted and rewritten rather than reconciled: they are short, they arrive
// complete, and reordering one changes every position anyway.
//
//nolint:gocritic // value param: this method conforms to the runtimeconfig.Store contract.
func (s *Store) SetRuntimeSettings(ctx context.Context, values runtimeconfig.Values) error {
	return s.withTx(ctx, "runtime settings", func(queries *sqlcgen.Queries) error {
		return s.writeRuntimeSettings(ctx, queries, &values)
	})
}

func (s *Store) writeRuntimeSettings(ctx context.Context, queries *sqlcgen.Queries, values *runtimeconfig.Values) error {
	if err := queries.UpdateRuntimeSettings(ctx, sqlcgen.UpdateRuntimeSettingsParams{
		AllowEmptySourceDeletion:      boolInteger(values.Sync.AllowEmptySourceDeletion),
		StaleAfterSeconds:             int64(values.Sync.StaleAfter / time.Second),
		SyncInitialDelaySeconds:       int64(values.Sync.InitialDelay / time.Second),
		NotificationsEnabled:          boolInteger(values.Notifications.Enabled),
		PushoverBaseUrl:               values.Notifications.PushoverBaseURL,
		SurfaceRebuildIntervalSeconds: int64(values.Surface.RebuildInterval / time.Second),
		WahooApiBaseUrl:               values.Wahoo.APIBaseURL, WahooOauthBaseUrl: values.Wahoo.OAuthBaseURL,
		WahooClientID: values.Wahoo.ClientID, RidemodelCoefficientsFile: values.RideModel.CoefficientsFile,
		Timezone: values.Timezone, UpdatedAtUnix: time.Now().Unix(),
	}); err != nil {
		return fmt.Errorf("storing the runtime settings: %w", err)
	}

	if err := queries.DeleteRuntimeBasemaps(ctx); err != nil {
		return fmt.Errorf("clearing the basemaps: %w", err)
	}
	for position, basemap := range values.Basemaps {
		if err := queries.InsertRuntimeBasemap(ctx, sqlcgen.InsertRuntimeBasemapParams{
			Position: int64(position), Name: basemap.Name, StyleUrl: basemap.StyleURL,
			StyleUrlDark: basemap.StyleURLDark, DarkCartography: boolInteger(basemap.DarkCartography),
		}); err != nil {
			return fmt.Errorf("storing a basemap: %w", err)
		}
	}

	if err := queries.DeleteRuntimeSurfaceRegions(ctx); err != nil {
		return fmt.Errorf("clearing the surface regions: %w", err)
	}
	for position, region := range values.Surface.Regions {
		if err := queries.InsertRuntimeSurfaceRegion(ctx, sqlcgen.InsertRuntimeSurfaceRegionParams{
			Position: int64(position), Region: region,
		}); err != nil {
			return fmt.Errorf("storing a surface region: %w", err)
		}
	}

	if err := queries.DeleteRuntimeSources(ctx); err != nil {
		return fmt.Errorf("clearing the sources: %w", err)
	}
	for position, source := range values.Sources {
		if err := queries.InsertRuntimeSource(ctx, sqlcgen.InsertRuntimeSourceParams{
			Position: int64(position), Provider: string(source.Provider), BaseUrl: source.BaseURL,
		}); err != nil {
			return fmt.Errorf("storing a source: %w", err)
		}
	}

	if err := queries.DeleteRuntimeTargets(ctx); err != nil {
		return fmt.Errorf("clearing the targets: %w", err)
	}
	for position, targetID := range values.Wahoo.Targets {
		if err := queries.InsertRuntimeTarget(ctx, sqlcgen.InsertRuntimeTargetParams{
			Position: int64(position), TargetID: targetID,
		}); err != nil {
			return fmt.Errorf("storing a target: %w", err)
		}
		// A newly named slot gets its durable record here rather than at the next
		// startup, so the OAuth onboarding that follows has a row to authorize. A
		// removed slot keeps its record, and nothing reads an unconfigured one.
		if err := queries.EnsureTarget(ctx, sqlcgen.EnsureTargetParams{
			Slot: targetID, AuthorizationState: string(AuthorizationNotAuthorized), UpdatedAtUnix: time.Now().Unix(),
		}); err != nil {
			return fmt.Errorf("creating target slot: %w", err)
		}
	}

	return nil
}

func (s *Store) runtimeBasemaps(ctx context.Context) ([]runtimeconfig.Basemap, error) {
	rows, err := s.queries.ListRuntimeBasemaps(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading the basemaps: %w", err)
	}
	basemaps := make([]runtimeconfig.Basemap, 0, len(rows))
	for _, row := range rows {
		basemaps = append(basemaps, runtimeconfig.Basemap{
			Name: row.Name, StyleURL: row.StyleUrl, StyleURLDark: row.StyleUrlDark,
			DarkCartography: row.DarkCartography != 0,
		})
	}

	return basemaps, nil
}

func (s *Store) runtimeTargets(ctx context.Context) ([]string, error) {
	rows, err := s.queries.ListRuntimeTargets(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading the targets: %w", err)
	}
	return rows, nil
}

func (s *Store) runtimeSources(ctx context.Context) ([]runtimeconfig.Source, error) {
	rows, err := s.queries.ListRuntimeSources(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading the sources: %w", err)
	}
	sources := make([]runtimeconfig.Source, 0, len(rows))
	for _, row := range rows {
		sources = append(sources, runtimeconfig.Source{Provider: route.Provider(row.Provider), BaseURL: row.BaseUrl})
	}

	return sources, nil
}

// RuntimeSecrets reads every stored credential. It satisfies runtimeconfig.Store.
// A ciphertext that will not open is a state failure rather than an absent
// secret: the database was written under a different encryption key.
func (s *Store) RuntimeSecrets(ctx context.Context) (map[runtimeconfig.SecretName]runtimeconfig.Secret, error) {
	rows, err := s.queries.ListRuntimeSecrets(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading the runtime secrets: %w", err)
	}
	secrets := make(map[runtimeconfig.SecretName]runtimeconfig.Secret)
	for _, row := range rows {
		value, decryptErr := s.decrypt(row.Name, row.Value)
		if decryptErr != nil {
			return nil, fmt.Errorf("reading runtime secret: %w", decryptErr)
		}
		secrets[runtimeconfig.SecretName(row.Name)] = runtimeconfig.NewSecret(value)
	}

	return secrets, nil
}

// SetRuntimeSecrets replaces only the credentials it is given, so a settings
// page can offer a replacement without ever having been told the current value.
// A secret carrying no bytes is removed. It satisfies runtimeconfig.Store.
func (s *Store) SetRuntimeSecrets(
	ctx context.Context, secrets map[runtimeconfig.SecretName]runtimeconfig.Secret,
) error {
	return s.withTx(ctx, "runtime secrets", func(queries *sqlcgen.Queries) error {
		return s.writeRuntimeSecrets(ctx, queries, secrets)
	})
}

func (s *Store) writeRuntimeSecrets(
	ctx context.Context, queries *sqlcgen.Queries, secrets map[runtimeconfig.SecretName]runtimeconfig.Secret,
) error {

	for name, secret := range secrets {
		if !secret.IsSet() {
			if err := queries.DeleteRuntimeSecret(ctx, string(name)); err != nil {
				return fmt.Errorf("clearing a runtime secret: %w", err)
			}
			continue
		}
		ciphertext, encryptErr := s.encrypt(string(name), secret.Bytes())
		if encryptErr != nil {
			return fmt.Errorf("encrypting a runtime secret: %w", encryptErr)
		}
		if err := queries.UpsertRuntimeSecret(ctx, sqlcgen.UpsertRuntimeSecretParams{
			Name: string(name), Value: ciphertext, UpdatedAtUnix: time.Now().Unix(),
		}); err != nil {
			return fmt.Errorf("storing a runtime secret: %w", err)
		}
	}

	return nil
}

// SetRuntimeSettingsAndSecrets commits a settings section and its credentials together.
//
//nolint:gocritic // value param: this method conforms to the runtimeconfig.Store contract.
func (s *Store) SetRuntimeSettingsAndSecrets(
	ctx context.Context, values runtimeconfig.Values, secrets map[runtimeconfig.SecretName]runtimeconfig.Secret,
) error {
	return s.withTx(ctx, "runtime settings and secrets", func(queries *sqlcgen.Queries) error {
		if err := s.writeRuntimeSettings(ctx, queries, &values); err != nil {
			return err
		}
		return s.writeRuntimeSecrets(ctx, queries, secrets)
	})
}

func (s *Store) runtimeSurfaceRegions(ctx context.Context) ([]string, error) {
	rows, err := s.queries.ListRuntimeSurfaceRegions(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading the surface regions: %w", err)
	}
	return rows, nil
}
