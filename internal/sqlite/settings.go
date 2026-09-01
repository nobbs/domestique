package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/runtimeconfig"
)

// RuntimeSettings reads the settings an operator edits while the service is
// running. It satisfies runtimeconfig.Store. Both lists come back in the order
// they were arranged in: the first basemap is what a browser loads by default.
func (s *Store) RuntimeSettings(ctx context.Context) (runtimeconfig.Values, error) {
	var (
		values                 runtimeconfig.Values
		staleAfterSeconds      int64
		initialDelaySeconds    int64
		rebuildIntervalSeconds int64
	)
	// success_policy and digest_interval_seconds are unused, superseded by the
	// per-task alert matrix; both are NOT NULL with CHECK, so dropping them needs a rebuild.
	if err := s.database.QueryRowContext(ctx, `
		SELECT allow_empty_source_deletion, stale_after_seconds, sync_initial_delay_seconds,
			notifications_enabled,
			pushover_base_url, surface_rebuild_interval_seconds,
			wahoo_api_base_url, wahoo_oauth_base_url, wahoo_client_id,
			ridemodel_coefficients_file, timezone
		FROM runtime_settings
		WHERE id = 1
	`).Scan(
		&values.Sync.AllowEmptySourceDeletion, &staleAfterSeconds, &initialDelaySeconds,
		&values.Notifications.Enabled,
		&values.Notifications.PushoverBaseURL, &rebuildIntervalSeconds,
		&values.Wahoo.APIBaseURL, &values.Wahoo.OAuthBaseURL, &values.Wahoo.ClientID,
		&values.RideModel.CoefficientsFile, &values.Timezone,
	); err != nil {
		return runtimeconfig.Values{}, fmt.Errorf("reading the runtime settings: %w", err)
	}
	values.Sync.StaleAfter = time.Duration(staleAfterSeconds) * time.Second
	values.Sync.InitialDelay = time.Duration(initialDelaySeconds) * time.Second
	values.Surface.RebuildInterval = time.Duration(rebuildIntervalSeconds) * time.Second

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
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting the runtime settings write: %w", err)
	}
	defer rollback(transaction)
	if err := s.writeRuntimeSettings(ctx, transaction, &values); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("committing the runtime settings: %w", err)
	}
	return nil
}

func (s *Store) writeRuntimeSettings(ctx context.Context, transaction *sql.Tx, values *runtimeconfig.Values) error {

	if _, err := transaction.ExecContext(ctx, `
		UPDATE runtime_settings
		SET allow_empty_source_deletion = ?, stale_after_seconds = ?, sync_initial_delay_seconds = ?,
			notifications_enabled = ?,
			pushover_base_url = ?, surface_rebuild_interval_seconds = ?,
			wahoo_api_base_url = ?, wahoo_oauth_base_url = ?, wahoo_client_id = ?,
			ridemodel_coefficients_file = ?, timezone = ?, updated_at_unix = ?
		WHERE id = 1
	`,
		values.Sync.AllowEmptySourceDeletion, int64(values.Sync.StaleAfter/time.Second),
		int64(values.Sync.InitialDelay/time.Second),
		values.Notifications.Enabled,
		values.Notifications.PushoverBaseURL,
		int64(values.Surface.RebuildInterval/time.Second),
		values.Wahoo.APIBaseURL, values.Wahoo.OAuthBaseURL, values.Wahoo.ClientID,
		values.RideModel.CoefficientsFile, values.Timezone,
		time.Now().Unix(),
	); err != nil {
		return fmt.Errorf("storing the runtime settings: %w", err)
	}

	if _, err := transaction.ExecContext(ctx, `DELETE FROM runtime_basemap`); err != nil {
		return fmt.Errorf("clearing the basemaps: %w", err)
	}
	for position, basemap := range values.Basemaps {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO runtime_basemap (position, name, style_url, style_url_dark, dark_cartography)
			VALUES (?, ?, ?, ?, ?)
		`, position, basemap.Name, basemap.StyleURL, basemap.StyleURLDark, basemap.DarkCartography); err != nil {
			return fmt.Errorf("storing a basemap: %w", err)
		}
	}

	if _, err := transaction.ExecContext(ctx, `DELETE FROM runtime_surface_region`); err != nil {
		return fmt.Errorf("clearing the surface regions: %w", err)
	}
	for position, region := range values.Surface.Regions {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO runtime_surface_region (position, region) VALUES (?, ?)
		`, position, region); err != nil {
			return fmt.Errorf("storing a surface region: %w", err)
		}
	}

	if _, err := transaction.ExecContext(ctx, `DELETE FROM runtime_source`); err != nil {
		return fmt.Errorf("clearing the sources: %w", err)
	}
	for position, source := range values.Sources {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO runtime_source (position, provider, base_url) VALUES (?, ?, ?)
		`, position, string(source.Provider), source.BaseURL); err != nil {
			return fmt.Errorf("storing a source: %w", err)
		}
	}

	if _, err := transaction.ExecContext(ctx, `DELETE FROM runtime_target`); err != nil {
		return fmt.Errorf("clearing the targets: %w", err)
	}
	for position, targetID := range values.Wahoo.Targets {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO runtime_target (position, target_id) VALUES (?, ?)
		`, position, targetID); err != nil {
			return fmt.Errorf("storing a target: %w", err)
		}
		// A newly named slot gets its durable record here rather than at the next
		// startup, so the OAuth onboarding that follows has a row to authorize. A
		// removed slot keeps its record, and nothing reads an unconfigured one.
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO targets (slot, authorization_state, updated_at_unix)
			VALUES (?, ?, ?)
			ON CONFLICT(slot) DO NOTHING
		`, targetID, AuthorizationNotAuthorized, time.Now().Unix()); err != nil {
			return fmt.Errorf("creating target slot: %w", err)
		}
	}

	return nil
}

func (s *Store) runtimeBasemaps(ctx context.Context) ([]runtimeconfig.Basemap, error) {
	rows, err := s.database.QueryContext(ctx, `
		SELECT name, style_url, style_url_dark, dark_cartography
		FROM runtime_basemap
		ORDER BY position
	`)
	if err != nil {
		return nil, fmt.Errorf("reading the basemaps: %w", err)
	}
	defer closeRows(rows)

	var basemaps []runtimeconfig.Basemap
	for rows.Next() {
		var basemap runtimeconfig.Basemap
		if err := rows.Scan(&basemap.Name, &basemap.StyleURL, &basemap.StyleURLDark, &basemap.DarkCartography); err != nil {
			return nil, fmt.Errorf("scanning a basemap: %w", err)
		}
		basemaps = append(basemaps, basemap)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the basemaps: %w", err)
	}

	return basemaps, nil
}

func (s *Store) runtimeTargets(ctx context.Context) ([]string, error) {
	rows, err := s.database.QueryContext(ctx, `
		SELECT target_id FROM runtime_target ORDER BY position
	`)
	if err != nil {
		return nil, fmt.Errorf("reading the targets: %w", err)
	}
	defer closeRows(rows)

	var targets []string
	for rows.Next() {
		var targetID string
		if err := rows.Scan(&targetID); err != nil {
			return nil, fmt.Errorf("scanning a target: %w", err)
		}
		targets = append(targets, targetID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the targets: %w", err)
	}

	return targets, nil
}

func (s *Store) runtimeSources(ctx context.Context) ([]runtimeconfig.Source, error) {
	rows, err := s.database.QueryContext(ctx, `
		SELECT provider, base_url FROM runtime_source ORDER BY position
	`)
	if err != nil {
		return nil, fmt.Errorf("reading the sources: %w", err)
	}
	defer closeRows(rows)

	var sources []runtimeconfig.Source
	for rows.Next() {
		var source runtimeconfig.Source
		if err := rows.Scan(&source.Provider, &source.BaseURL); err != nil {
			return nil, fmt.Errorf("scanning a source: %w", err)
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the sources: %w", err)
	}

	return sources, nil
}

// RuntimeSecrets reads every stored credential. It satisfies runtimeconfig.Store.
// A ciphertext that will not open is a state failure rather than an absent
// secret: the database was written under a different encryption key.
func (s *Store) RuntimeSecrets(ctx context.Context) (map[runtimeconfig.SecretName]runtimeconfig.Secret, error) {
	rows, err := s.database.QueryContext(ctx, `SELECT name, value FROM runtime_secret`)
	if err != nil {
		return nil, fmt.Errorf("reading the runtime secrets: %w", err)
	}
	defer closeRows(rows)

	secrets := make(map[runtimeconfig.SecretName]runtimeconfig.Secret)
	for rows.Next() {
		var (
			name       string
			ciphertext []byte
		)
		if err := rows.Scan(&name, &ciphertext); err != nil {
			return nil, fmt.Errorf("scanning a runtime secret: %w", err)
		}
		value, decryptErr := s.decrypt(name, ciphertext)
		if decryptErr != nil {
			return nil, fmt.Errorf("reading runtime secret: %w", decryptErr)
		}
		secrets[runtimeconfig.SecretName(name)] = runtimeconfig.NewSecret(value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the runtime secrets: %w", err)
	}

	return secrets, nil
}

// SetRuntimeSecrets replaces only the credentials it is given, so a settings
// page can offer a replacement without ever having been told the current value.
// A secret carrying no bytes is removed. It satisfies runtimeconfig.Store.
func (s *Store) SetRuntimeSecrets(
	ctx context.Context, secrets map[runtimeconfig.SecretName]runtimeconfig.Secret,
) error {
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting the runtime secrets write: %w", err)
	}
	defer rollback(transaction)
	if err := s.writeRuntimeSecrets(ctx, transaction, secrets); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("committing the runtime secrets: %w", err)
	}
	return nil
}

func (s *Store) writeRuntimeSecrets(
	ctx context.Context, transaction *sql.Tx, secrets map[runtimeconfig.SecretName]runtimeconfig.Secret,
) error {

	for name, secret := range secrets {
		if !secret.IsSet() {
			if _, err := transaction.ExecContext(ctx,
				`DELETE FROM runtime_secret WHERE name = ?`, string(name)); err != nil {
				return fmt.Errorf("clearing a runtime secret: %w", err)
			}
			continue
		}
		ciphertext, encryptErr := s.encrypt(string(name), secret.Bytes())
		if encryptErr != nil {
			return fmt.Errorf("encrypting a runtime secret: %w", encryptErr)
		}
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO runtime_secret (name, value, updated_at_unix)
			VALUES (?, ?, ?)
			ON CONFLICT(name) DO UPDATE SET value = excluded.value,
				updated_at_unix = excluded.updated_at_unix
		`, string(name), ciphertext, time.Now().Unix()); err != nil {
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
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting the runtime settings write: %w", err)
	}
	defer rollback(transaction)
	if err := s.writeRuntimeSettings(ctx, transaction, &values); err != nil {
		return err
	}
	if err := s.writeRuntimeSecrets(ctx, transaction, secrets); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("committing runtime settings and secrets: %w", err)
	}
	return nil
}

func (s *Store) runtimeSurfaceRegions(ctx context.Context) ([]string, error) {
	rows, err := s.database.QueryContext(ctx, `
		SELECT region FROM runtime_surface_region ORDER BY position
	`)
	if err != nil {
		return nil, fmt.Errorf("reading the surface regions: %w", err)
	}
	defer closeRows(rows)

	var regions []string
	for rows.Next() {
		var region string
		if err := rows.Scan(&region); err != nil {
			return nil, fmt.Errorf("scanning a surface region: %w", err)
		}
		regions = append(regions, region)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the surface regions: %w", err)
	}

	return regions, nil
}
