package sqlite

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
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

	queries := s.queries.WithTx(transaction)
	_, err = queries.FindTargetByWahooUser(ctx, sqlcgen.FindTargetByWahooUserParams{
		WahooUserID: sql.NullString{String: wahooUserID, Valid: true}, Slot: targetID,
	})
	if err == nil {
		return ErrWahooUserAlreadyAuthorized
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("checking existing target authorization: %w", err)
	}

	result, err := queries.UpdateTargetAuthorization(ctx, sqlcgen.UpdateTargetAuthorizationParams{
		WahooUserID: sql.NullString{String: wahooUserID, Valid: true}, RefreshToken: encryptedToken,
		AuthorizationState: string(AuthorizationAuthorized), UpdatedAtUnix: time.Now().Unix(), Slot: targetID,
	})
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
	encryptedToken, err := s.queries.GetRefreshToken(ctx, targetID)
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

	result, err := s.queries.UpdateRefreshToken(ctx, sqlcgen.UpdateRefreshTokenParams{
		RefreshToken: encryptedToken, AuthorizationState: string(AuthorizationAuthorized),
		UpdatedAtUnix: time.Now().Unix(), Slot: targetID,
	})
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
	result, err := s.queries.MarkTargetNeedsReauthorization(ctx, sqlcgen.MarkTargetNeedsReauthorizationParams{
		AuthorizationState: string(AuthorizationNeedsReauthorization), UpdatedAtUnix: time.Now().Unix(), Slot: targetID,
	})
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

	queries := s.queries.WithTx(transaction)
	targetExists, err := queries.TargetExists(ctx, targetID)
	if err != nil {
		return fmt.Errorf("checking oauth target: %w", err)
	}
	if targetExists == 0 {
		return ErrTargetNotFound
	}

	now := time.Now().Unix()
	if err := queries.DeletePriorOAuthTransactions(ctx, sqlcgen.DeletePriorOAuthTransactionsParams{
		ExpiresAtUnix: now, TargetSlot: targetID, CallerLogin: callerLogin,
	}); err != nil {
		return fmt.Errorf("clearing prior oauth transactions: %w", err)
	}
	if err := queries.InsertOAuthTransaction(ctx, sqlcgen.InsertOAuthTransactionParams{
		ID: hex.EncodeToString(stateDigest), TargetSlot: targetID, StateDigest: stateDigest,
		CodeVerifier: []byte{}, ExpiresAtUnix: expiresAt.Unix(), CallerLogin: callerLogin,
	}); err != nil {
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

	queries := s.queries.WithTx(transaction)
	row, err := queries.GetOAuthTransaction(ctx, stateDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrOAuthTransactionNotFound
	}
	if err != nil {
		return "", fmt.Errorf("reading oauth transaction: %w", err)
	}
	if row.CallerLogin != callerLogin {
		return "", ErrOAuthTransactionIdentityMismatch
	}
	if row.UsedAtUnix.Valid {
		return "", ErrOAuthTransactionUsed
	}
	if row.ExpiresAtUnix <= time.Now().Unix() {
		return "", ErrOAuthTransactionExpired
	}

	result, err := queries.ConsumeOAuthTransaction(ctx, sqlcgen.ConsumeOAuthTransactionParams{
		UsedAtUnix: sql.NullInt64{Int64: time.Now().Unix(), Valid: true}, StateDigest: stateDigest,
	})
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

	return row.TargetSlot, nil
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
	rows, err := s.queries.ListPendingAuthorizations(ctx, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("listing pending authorizations: %w", err)
	}
	for _, targetID := range rows {
		if err := visit(targetID); err != nil {
			return err
		}
	}

	return nil
}
