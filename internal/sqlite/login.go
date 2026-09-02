package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
)

// maximumLoginTransactions bounds login_transactions against flood from its
// publicly reachable endpoint: the table must self-heal rather than grow.
const maximumLoginTransactions = 64

// BeginLogin saves a hashed, expiring browser sign-in state together with its
// nonce and PKCE code verifier. The raw state value is never persisted.
// Expired rows are cleared first, then the table is capped at
// maximumLoginTransactions by evicting the oldest beyond that bound.
func (s *Store) BeginLogin(ctx context.Context, stateDigest []byte, nonce, codeVerifier string, now, expiresAt time.Time) error {
	if len(stateDigest) != 32 || strings.TrimSpace(nonce) == "" || strings.TrimSpace(codeVerifier) == "" || !expiresAt.After(now) {
		return errors.New("state digest, nonce, code verifier, and future expiry are required")
	}

	return s.withTx(ctx, "login transaction", func(queries *sqlcgen.Queries) error {
		if err := queries.DeleteExpiredLoginTransactions(ctx, now.Unix()); err != nil {
			return fmt.Errorf("clearing expired login transactions: %w", err)
		}
		if err := queries.CapLoginTransactions(ctx, maximumLoginTransactions-1); err != nil {
			return fmt.Errorf("capping login transactions: %w", err)
		}
		if err := queries.InsertLoginTransaction(ctx, sqlcgen.InsertLoginTransactionParams{
			StateDigest: stateDigest, Nonce: nonce, CodeVerifier: codeVerifier, ExpiresAtUnix: expiresAt.Unix(),
		}); err != nil {
			return fmt.Errorf("storing login transaction: %w", err)
		}
		return nil
	})
}

// ConsumeLogin reads and deletes a pending sign-in state in one transaction:
// the delete is the consume, so a state can be used at most once.
func (s *Store) ConsumeLogin(ctx context.Context, stateDigest []byte, now time.Time) (nonce, codeVerifier string, err error) {
	if len(stateDigest) != 32 {
		return "", "", errors.New("state digest is required")
	}

	var expires int64
	txErr := s.withTx(ctx, "login consumption", func(queries *sqlcgen.Queries) error {
		row, getErr := queries.GetLoginTransaction(ctx, stateDigest)
		if errors.Is(getErr, sql.ErrNoRows) {
			return ErrLoginNotFound
		}
		if getErr != nil {
			return fmt.Errorf("reading login transaction: %w", getErr)
		}
		nonce, codeVerifier, expires = row.Nonce, row.CodeVerifier, row.ExpiresAtUnix
		if err := queries.DeleteLoginTransaction(ctx, stateDigest); err != nil {
			return fmt.Errorf("consuming login transaction: %w", err)
		}
		return nil
	})
	if txErr != nil {
		return "", "", txErr
	}
	if expires <= now.Unix() {
		return "", "", ErrLoginExpired
	}

	return nonce, codeVerifier, nil
}

// CreateSession saves a hashed, expiring web session. The raw token is never
// persisted. Expired sessions are cleared first.
func (s *Store) CreateSession(ctx context.Context, tokenDigest []byte, subject, display string, now, expiresAt time.Time) error {
	if len(tokenDigest) != 32 || strings.TrimSpace(subject) == "" || strings.TrimSpace(display) == "" || !expiresAt.After(now) {
		return errors.New("token digest, subject, display, and future expiry are required")
	}

	return s.withTx(ctx, "web session", func(queries *sqlcgen.Queries) error {
		if err := queries.DeleteExpiredWebSessions(ctx, now.Unix()); err != nil {
			return fmt.Errorf("clearing expired web sessions: %w", err)
		}
		if err := queries.InsertWebSession(ctx, sqlcgen.InsertWebSessionParams{
			TokenDigest: tokenDigest, Subject: subject, Display: display,
			CreatedAtUnix: now.Unix(), RenewedAtUnix: now.Unix(), ExpiresAtUnix: expiresAt.Unix(),
		}); err != nil {
			return fmt.Errorf("storing web session: %w", err)
		}
		return nil
	})
}

// Session returns a web session's identity and lifetime by its hashed token.
// It leaves an expired row in place: a write path prunes it instead, so a
// read alone never mutates state.
func (s *Store) Session(ctx context.Context, tokenDigest []byte, now time.Time) (subject, display string, renewedAt, expiresAt time.Time, err error) {
	if len(tokenDigest) != 32 {
		return "", "", time.Time{}, time.Time{}, errors.New("token digest is required")
	}

	row, err := s.queries.GetWebSession(ctx, tokenDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", time.Time{}, time.Time{}, ErrSessionNotFound
	}
	if err != nil {
		return "", "", time.Time{}, time.Time{}, fmt.Errorf("reading web session: %w", err)
	}
	if row.ExpiresAtUnix <= now.Unix() {
		return "", "", time.Time{}, time.Time{}, ErrSessionExpired
	}

	return row.Subject, row.Display, time.Unix(row.RenewedAtUnix, 0).UTC(), time.Unix(row.ExpiresAtUnix, 0).UTC(), nil
}

// RenewSession extends a web session's lifetime and prunes other expired
// sessions in the same transaction.
func (s *Store) RenewSession(ctx context.Context, tokenDigest []byte, now, expiresAt time.Time) error {
	if len(tokenDigest) != 32 || !expiresAt.After(now) {
		return errors.New("token digest and future expiry are required")
	}

	return s.withTx(ctx, "web session renewal", func(queries *sqlcgen.Queries) error {
		if err := queries.DeleteExpiredWebSessions(ctx, now.Unix()); err != nil {
			return fmt.Errorf("clearing expired web sessions: %w", err)
		}
		result, err := queries.RenewWebSession(ctx, sqlcgen.RenewWebSessionParams{
			RenewedAtUnix: now.Unix(), ExpiresAtUnix: expiresAt.Unix(), TokenDigest: tokenDigest,
		})
		if err != nil {
			return fmt.Errorf("renewing web session: %w", err)
		}
		updated, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("checking web session renewal: %w", err)
		}
		if updated == 0 {
			return ErrSessionNotFound
		}
		return nil
	})
}

// DeleteSession removes a web session by its hashed token. Deleting a token
// that is not there is not an error: the caller's goal, that it be gone, is
// already met.
func (s *Store) DeleteSession(ctx context.Context, tokenDigest []byte) error {
	if len(tokenDigest) != 32 {
		return errors.New("token digest is required")
	}
	if err := s.queries.DeleteWebSession(ctx, tokenDigest); err != nil {
		return fmt.Errorf("deleting web session: %w", err)
	}

	return nil
}
