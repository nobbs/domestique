package sqlite

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
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

// TargetAuthorization returns the durable authorization state for one target
// without exposing its Wahoo identity or refresh token.
func (s *Store) TargetAuthorization(ctx context.Context, targetID string) (string, error) {
	target, err := s.Target(ctx, targetID)
	if err != nil {
		return "", err
	}

	return string(target.AuthorizationState), nil
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
// slot and one caller identity. The raw state value is never persisted.
func (s *Store) BeginAuthorization(
	ctx context.Context,
	targetID, callerLogin string,
	stateDigest []byte,
	expiresAt time.Time,
) error {
	if strings.TrimSpace(targetID) == "" || strings.TrimSpace(callerLogin) == "" ||
		len(stateDigest) != 32 || !expiresAt.After(time.Now()) {
		return errors.New("target ID, caller identity, state digest, and future expiry are required")
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
	`, now, targetID, callerLogin); err != nil {
		return fmt.Errorf("clearing prior oauth transactions: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO oauth_transactions (
			id, target_slot, state_digest, code_verifier, expires_at_unix, caller_login
		) VALUES (?, ?, ?, ?, ?, ?)
	`, hex.EncodeToString(stateDigest), targetID, stateDigest, []byte{}, expiresAt.Unix(), callerLogin); err != nil {
		return fmt.Errorf("storing oauth transaction: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("committing oauth transaction: %w", err)
	}

	return nil
}

// ConsumeAuthorization verifies and marks a pending OAuth state used. It
// returns the bound target slot, never the raw state or caller identity.
func (s *Store) ConsumeAuthorization(ctx context.Context, callerLogin string, stateDigest []byte) (string, error) {
	if strings.TrimSpace(callerLogin) == "" || len(stateDigest) != 32 {
		return "", errors.New("caller identity and state digest are required")
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
	if caller != callerLogin {
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

// ForEachPendingAuthorization visits every target slot with an authorization in
// flight: a transaction that has neither expired nor been consumed. "pending" is
// a state of the flow rather than of the slot, so it is this row's presence
// rather than a fourth stored state needing transitions nothing reports. It
// reports the slot alone; the digest, identity and expiry never leave here.
func (s *Store) ForEachPendingAuthorization(ctx context.Context, visit func(targetID string) error) error {
	if visit == nil {
		return errors.New("pending authorization visitor is required")
	}
	rows, err := s.database.QueryContext(ctx, `
		SELECT DISTINCT target_slot
		FROM oauth_transactions
		WHERE used_at_unix IS NULL AND expires_at_unix > ?
		ORDER BY target_slot
	`, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("listing pending authorizations: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var targetID string
		if err := rows.Scan(&targetID); err != nil {
			return fmt.Errorf("reading pending authorization: %w", err)
		}
		if err := visit(targetID); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating pending authorizations: %w", err)
	}

	return nil
}
