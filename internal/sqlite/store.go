// Package sqlite persists Domestique's encrypted, local reconciliation state.
package sqlite

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite driver registration.
)

const driverName = "sqlite"

var (
	// ErrTargetNotFound reports an operation for an unknown configured target slot.
	ErrTargetNotFound = errors.New("target slot not found")
	// ErrRefreshTokenUnavailable reports a target that needs an authorization flow.
	ErrRefreshTokenUnavailable = errors.New("refresh token is unavailable")
	// ErrWahooUserAlreadyAuthorized reports an account already bound to another slot.
	ErrWahooUserAlreadyAuthorized = errors.New("wahoo user is already authorized for another target slot")
	// ErrStateUnreadable reports encrypted state that cannot be authenticated.
	ErrStateUnreadable = errors.New("encrypted state is unreadable")
	// ErrOAuthTransactionNotFound reports an unknown OAuth callback state.
	ErrOAuthTransactionNotFound = errors.New("oauth transaction was not found")
	// ErrOAuthTransactionExpired reports a callback state that exceeded its deadline.
	ErrOAuthTransactionExpired = errors.New("oauth transaction has expired")
	// ErrOAuthTransactionUsed reports a callback state that was already consumed.
	ErrOAuthTransactionUsed = errors.New("oauth transaction was already used")
	// ErrOAuthTransactionIdentityMismatch reports a callback from another Tailnet user.
	ErrOAuthTransactionIdentityMismatch = errors.New("oauth transaction identity did not match")
)

// AuthorizationState identifies a target slot's durable OAuth state.
type AuthorizationState string

const (
	// AuthorizationNotAuthorized means the target has not completed OAuth.
	AuthorizationNotAuthorized AuthorizationState = "not_authorized"
	// AuthorizationAuthorized means the target has a usable refresh token.
	AuthorizationAuthorized AuthorizationState = "authorized"
	// AuthorizationNeedsReauthorization means token refresh failed permanently.
	AuthorizationNeedsReauthorization AuthorizationState = "needs_reauthorization"
)

// Target is durable, non-secret state for one configured Wahoo target slot.
type Target struct {
	ID                 string
	WahooUserID        string
	AuthorizationState AuthorizationState
}

// Store is an SQLite-backed state store whose OAuth tokens use AES-GCM at rest.
type Store struct {
	database *sql.DB
	aead     cipher.AEAD
}

// Open creates or opens a state database, applies pending migrations, and
// configures encrypted token storage. The database path must be absolute.
func Open(ctx context.Context, databasePath string, encryptionKey [32]byte) (*Store, error) {
	if !filepath.IsAbs(databasePath) {
		return nil, errors.New("database path must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		return nil, fmt.Errorf("creating state directory: %w", err)
	}

	block, err := aes.NewCipher(encryptionKey[:])
	if err != nil {
		return nil, fmt.Errorf("creating state cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating state cipher: %w", err)
	}

	database, err := sql.Open(driverName, databasePath)
	if err != nil {
		return nil, fmt.Errorf("opening state database: %w", err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)

	store := &Store{database: database, aead: aead}
	if err := store.configure(ctx); err != nil {
		closeDatabase(database)
		return nil, err
	}
	if err := store.migrate(ctx); err != nil {
		closeDatabase(database)
		return nil, err
	}
	if err := os.Chmod(databasePath, 0o600); err != nil {
		closeDatabase(database)
		return nil, fmt.Errorf("protecting state database: %w", err)
	}

	return store, nil
}

// Close releases the database connection.
func (s *Store) Close() error {
	if err := s.database.Close(); err != nil {
		return fmt.Errorf("closing state database: %w", err)
	}

	return nil
}

// EnsureTargets creates durable records for configured target slots. It never
// removes a target, preserving state until an explicit migration is designed.
func (s *Store) EnsureTargets(ctx context.Context, targetIDs []string) error {
	if err := validateTargetIDs(targetIDs); err != nil {
		return err
	}

	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting target initialization: %w", err)
	}
	defer rollback(transaction)

	for _, targetID := range targetIDs {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO targets (slot, authorization_state, updated_at_unix)
			VALUES (?, ?, ?)
			ON CONFLICT(slot) DO NOTHING
		`, targetID, AuthorizationNotAuthorized, time.Now().Unix()); err != nil {
			return fmt.Errorf("creating target slot: %w", err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("committing target initialization: %w", err)
	}

	return nil
}

// Targets returns all target slots without exposing their refresh tokens.
func (s *Store) Targets(ctx context.Context) ([]Target, error) {
	rows, err := s.database.QueryContext(ctx, `
		SELECT slot, COALESCE(wahoo_user_id, ''), authorization_state
		FROM targets
		ORDER BY slot
	`)
	if err != nil {
		return nil, fmt.Errorf("listing target slots: %w", err)
	}
	defer closeRows(rows)

	var targets []Target
	for rows.Next() {
		var target Target
		if err := rows.Scan(&target.ID, &target.WahooUserID, &target.AuthorizationState); err != nil {
			return nil, fmt.Errorf("reading target slot: %w", err)
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating target slots: %w", err)
	}

	return targets, nil
}

// Target returns one target slot without exposing its refresh token.
func (s *Store) Target(ctx context.Context, targetID string) (Target, error) {
	var target Target
	err := s.database.QueryRowContext(ctx, `
		SELECT slot, COALESCE(wahoo_user_id, ''), authorization_state
		FROM targets
		WHERE slot = ?
	`, targetID).Scan(&target.ID, &target.WahooUserID, &target.AuthorizationState)
	if errors.Is(err, sql.ErrNoRows) {
		return Target{}, ErrTargetNotFound
	}
	if err != nil {
		return Target{}, fmt.Errorf("reading target slot: %w", err)
	}

	return target, nil
}

// AuthorizeTarget atomically binds a Wahoo user and encrypted refresh token to
// a configured target. One Wahoo user cannot authorize more than one slot.
func (s *Store) AuthorizeTarget(ctx context.Context, targetID, wahooUserID, refreshToken string) error {
	if strings.TrimSpace(targetID) == "" || strings.TrimSpace(wahooUserID) == "" || refreshToken == "" {
		return errors.New("target ID, Wahoo user ID, and refresh token are required")
	}

	encryptedToken, err := s.encrypt(targetID, []byte(refreshToken))
	if err != nil {
		return fmt.Errorf("encrypting refresh token: %w", err)
	}

	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting target authorization: %w", err)
	}
	defer rollback(transaction)

	var existingTargetID string
	err = transaction.QueryRowContext(ctx, `
		SELECT slot
		FROM targets
		WHERE wahoo_user_id = ? AND slot != ?
	`, wahooUserID, targetID).Scan(&existingTargetID)
	if err == nil {
		return ErrWahooUserAlreadyAuthorized
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("checking existing target authorization: %w", err)
	}

	result, err := transaction.ExecContext(ctx, `
		UPDATE targets
		SET wahoo_user_id = ?, refresh_token = ?, authorization_state = ?, updated_at_unix = ?
		WHERE slot = ?
	`, wahooUserID, encryptedToken, AuthorizationAuthorized, time.Now().Unix(), targetID)
	if err != nil {
		return fmt.Errorf("storing target authorization: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking target authorization: %w", err)
	}
	if updated == 0 {
		return ErrTargetNotFound
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("committing target authorization: %w", err)
	}

	return nil
}

// RefreshToken returns the decrypted refresh token for a configured target.
func (s *Store) RefreshToken(ctx context.Context, targetID string) (string, error) {
	var encryptedToken []byte
	err := s.database.QueryRowContext(ctx, `
		SELECT refresh_token
		FROM targets
		WHERE slot = ?
	`, targetID).Scan(&encryptedToken)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrTargetNotFound
	}
	if err != nil {
		return "", fmt.Errorf("reading refresh token: %w", err)
	}
	if len(encryptedToken) == 0 {
		return "", ErrRefreshTokenUnavailable
	}

	decryptedToken, err := s.decrypt(targetID, encryptedToken)
	if err != nil {
		return "", err
	}

	return string(decryptedToken), nil
}

// ReplaceRefreshToken atomically stores the refresh token returned by a
// successful Wahoo refresh. The replacement happens before another API request
// can use the prior token.
func (s *Store) ReplaceRefreshToken(ctx context.Context, targetID, refreshToken string) error {
	if strings.TrimSpace(targetID) == "" || refreshToken == "" {
		return errors.New("target ID and refresh token are required")
	}

	encryptedToken, err := s.encrypt(targetID, []byte(refreshToken))
	if err != nil {
		return fmt.Errorf("encrypting refresh token: %w", err)
	}

	result, err := s.database.ExecContext(ctx, `
		UPDATE targets
		SET refresh_token = ?, authorization_state = ?, updated_at_unix = ?
		WHERE slot = ?
	`, encryptedToken, AuthorizationAuthorized, time.Now().Unix(), targetID)
	if err != nil {
		return fmt.Errorf("replacing refresh token: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking refreshed target: %w", err)
	}
	if updated == 0 {
		return ErrTargetNotFound
	}

	return nil
}

// MarkNeedsReauthorization clears a target's refresh token after a permanent
// OAuth failure and leaves it ready for a fresh interactive authorization.
func (s *Store) MarkNeedsReauthorization(ctx context.Context, targetID string) error {
	result, err := s.database.ExecContext(ctx, `
		UPDATE targets
		SET refresh_token = NULL, authorization_state = ?, updated_at_unix = ?
		WHERE slot = ?
	`, AuthorizationNeedsReauthorization, time.Now().Unix(), targetID)
	if err != nil {
		return fmt.Errorf("marking target for reauthorization: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking reauthorization update: %w", err)
	}
	if updated == 0 {
		return ErrTargetNotFound
	}

	return nil
}

// BeginAuthorization saves a hashed, expiring OAuth state bound to one target
// slot and one Tailnet identity. The raw state value is never persisted.
func (s *Store) BeginAuthorization(
	ctx context.Context,
	targetID, tailnetUserLogin string,
	stateDigest []byte,
	expiresAt time.Time,
) error {
	if strings.TrimSpace(targetID) == "" || strings.TrimSpace(tailnetUserLogin) == "" ||
		len(stateDigest) != 32 || !expiresAt.After(time.Now()) {
		return errors.New("target ID, Tailnet identity, state digest, and future expiry are required")
	}

	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting oauth transaction: %w", err)
	}
	defer rollback(transaction)

	var targetExists bool
	if err := transaction.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM targets WHERE slot = ?)`, targetID).Scan(&targetExists); err != nil {
		return fmt.Errorf("checking oauth target: %w", err)
	}
	if !targetExists {
		return ErrTargetNotFound
	}

	now := time.Now().Unix()
	if _, err := transaction.ExecContext(ctx, `
		DELETE FROM oauth_transactions
		WHERE expires_at_unix <= ? OR (target_slot = ? AND caller_login = ? AND used_at_unix IS NULL)
	`, now, targetID, tailnetUserLogin); err != nil {
		return fmt.Errorf("clearing prior oauth transactions: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO oauth_transactions (
			id, target_slot, state_digest, code_verifier, expires_at_unix, caller_login
		) VALUES (?, ?, ?, ?, ?, ?)
	`, hex.EncodeToString(stateDigest), targetID, stateDigest, []byte{}, expiresAt.Unix(), tailnetUserLogin); err != nil {
		return fmt.Errorf("storing oauth transaction: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("committing oauth transaction: %w", err)
	}

	return nil
}

// ConsumeAuthorization verifies and marks a pending OAuth state used. It
// returns the bound target slot, never the raw state or caller identity.
func (s *Store) ConsumeAuthorization(ctx context.Context, tailnetUserLogin string, stateDigest []byte) (string, error) {
	if strings.TrimSpace(tailnetUserLogin) == "" || len(stateDigest) != 32 {
		return "", errors.New("tailnet identity and state digest are required")
	}

	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("starting oauth callback transaction: %w", err)
	}
	defer rollback(transaction)

	var (
		targetID string
		caller   string
		expires  int64
		usedAt   sql.NullInt64
	)
	err = transaction.QueryRowContext(ctx, `
		SELECT target_slot, caller_login, expires_at_unix, used_at_unix
		FROM oauth_transactions
		WHERE state_digest = ?
	`, stateDigest).Scan(&targetID, &caller, &expires, &usedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrOAuthTransactionNotFound
	}
	if err != nil {
		return "", fmt.Errorf("reading oauth transaction: %w", err)
	}
	if caller != tailnetUserLogin {
		return "", ErrOAuthTransactionIdentityMismatch
	}
	if usedAt.Valid {
		return "", ErrOAuthTransactionUsed
	}
	if expires <= time.Now().Unix() {
		return "", ErrOAuthTransactionExpired
	}

	result, err := transaction.ExecContext(ctx, `
		UPDATE oauth_transactions
		SET used_at_unix = ?
		WHERE state_digest = ? AND used_at_unix IS NULL
	`, time.Now().Unix(), stateDigest)
	if err != nil {
		return "", fmt.Errorf("consuming oauth transaction: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("checking oauth transaction consumption: %w", err)
	}
	if updated == 0 {
		return "", ErrOAuthTransactionUsed
	}
	if err := transaction.Commit(); err != nil {
		return "", fmt.Errorf("committing oauth callback transaction: %w", err)
	}

	return targetID, nil
}

func (s *Store) configure(ctx context.Context) error {
	if err := s.database.PingContext(ctx); err != nil {
		return fmt.Errorf("opening state database: %w", err)
	}
	for _, statement := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
	} {
		if _, err := s.database.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configuring state database: %w", err)
		}
	}

	return nil
}

func (s *Store) migrate(ctx context.Context) error {
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting state migration: %w", err)
	}
	defer rollback(transaction)

	if _, err := transaction.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at_unix INTEGER NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("creating migration registry: %w", err)
	}

	var currentVersion int
	if err := transaction.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&currentVersion); err != nil {
		return fmt.Errorf("reading state schema version: %w", err)
	}
	migrations := schemaMigrations()
	if currentVersion > len(migrations) {
		return errors.New("state schema version is newer than this service")
	}

	for version := currentVersion + 1; version <= len(migrations); version++ {
		for _, statement := range migrations[version-1] {
			if _, err := transaction.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("applying state migration %d: %w", version, err)
			}
		}
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO schema_migrations (version, applied_at_unix) VALUES (?, ?)
		`, version, time.Now().Unix()); err != nil {
			return fmt.Errorf("recording state migration %d: %w", version, err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("committing state migration: %w", err)
	}

	return nil
}

func (s *Store) encrypt(targetID string, value []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("creating encryption nonce: %w", err)
	}

	return s.aead.Seal(nonce, nonce, value, []byte(targetID)), nil
}

func (s *Store) decrypt(targetID string, value []byte) ([]byte, error) {
	nonceSize := s.aead.NonceSize()
	if len(value) <= nonceSize {
		return nil, ErrStateUnreadable
	}
	decrypted, err := s.aead.Open(nil, value[:nonceSize], value[nonceSize:], []byte(targetID))
	if err != nil {
		return nil, ErrStateUnreadable
	}

	return decrypted, nil
}

func validateTargetIDs(targetIDs []string) error {
	if len(targetIDs) == 0 {
		return errors.New("at least one target ID is required")
	}

	seen := make(map[string]struct{}, len(targetIDs))
	for _, targetID := range targetIDs {
		if strings.TrimSpace(targetID) == "" {
			return errors.New("target ID is required")
		}
		if _, found := seen[targetID]; found {
			return fmt.Errorf("target ID %q is duplicated", targetID)
		}
		seen[targetID] = struct{}{}
	}

	return nil
}

//nolint:errcheck // A cleanup error cannot replace the already-returned state error.
func closeDatabase(database *sql.DB) {
	_ = database.Close()
}

//nolint:errcheck // A query cleanup error is superseded by the query result.
func closeRows(rows *sql.Rows) {
	_ = rows.Close()
}

//nolint:errcheck // Rollback after a committed transaction is expected to return sql.ErrTxDone.
func rollback(transaction *sql.Tx) {
	_ = transaction.Rollback()
}

func schemaMigrations() [][]string {
	return [][]string{
		{
			`CREATE TABLE targets (
			slot TEXT PRIMARY KEY,
			wahoo_user_id TEXT UNIQUE,
			refresh_token BLOB,
			authorization_state TEXT NOT NULL CHECK (authorization_state IN ('not_authorized', 'authorized', 'needs_reauthorization')),
			updated_at_unix INTEGER NOT NULL
		)`,
			`CREATE TABLE oauth_transactions (
			id TEXT PRIMARY KEY,
			target_slot TEXT NOT NULL REFERENCES targets(slot),
			state_digest BLOB NOT NULL,
			code_verifier BLOB NOT NULL,
			expires_at_unix INTEGER NOT NULL,
			used_at_unix INTEGER
		)`,
			`CREATE TABLE source_stages (
			route_id INTEGER NOT NULL,
			stage_order INTEGER NOT NULL,
			source_revision TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			PRIMARY KEY (route_id, stage_order)
		)`,
			`CREATE TABLE target_stages (
			target_slot TEXT NOT NULL REFERENCES targets(slot),
			route_id INTEGER NOT NULL,
			stage_order INTEGER NOT NULL,
			wahoo_route_id INTEGER NOT NULL,
			content_hash TEXT NOT NULL,
			PRIMARY KEY (target_slot, route_id, stage_order)
		)`,
			`CREATE TABLE trusted_inventory (
			target_slot TEXT PRIMARY KEY REFERENCES targets(slot),
			captured_at_unix INTEGER NOT NULL
		)`,
			`CREATE TABLE trusted_inventory_stages (
			target_slot TEXT NOT NULL REFERENCES trusted_inventory(target_slot),
			route_id INTEGER NOT NULL,
			stage_order INTEGER NOT NULL,
			wahoo_route_id INTEGER NOT NULL,
			PRIMARY KEY (target_slot, route_id, stage_order)
		)`,
			`CREATE TABLE sync_runs (
			id INTEGER PRIMARY KEY,
			started_at_unix INTEGER NOT NULL,
			finished_at_unix INTEGER,
			outcome TEXT NOT NULL,
			detail TEXT
		)`,
			`CREATE TABLE notification_state (
			kind TEXT PRIMARY KEY,
			last_sent_at_unix INTEGER NOT NULL
		)`,
		},
		{
			`ALTER TABLE oauth_transactions ADD COLUMN caller_login TEXT NOT NULL DEFAULT ''`,
			`CREATE UNIQUE INDEX oauth_transactions_state_digest_index ON oauth_transactions(state_digest)`,
		},
	}
}
