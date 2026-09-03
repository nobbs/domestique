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

// Target is durable, non-secret state for one self-service Wahoo target.
type Target struct {
	ID                 string
	WahooUserID        string
	OwnerSubject       string
	AuthorizationState AuthorizationState
}

// EnsureTargetOwner creates a target's durable record if this is the first
// time its owning subject has been seen, and leaves an existing one alone.
// A self-service target's slot is the owning subject's own value, so this is
// safe to call on every "Connect" attempt, not only the first.
//
// A slot that predates ownership (owner_subject NULL, from migration 000030)
// is claimed rather than left orphaned forever: matching on slot here is
// never a guess, since a self-service slot IS the owning subject's own
// value. This is a no-op against a slot already owned by someone — including
// this same subject, on their second or later "Connect".
func (s *Store) EnsureTargetOwner(ctx context.Context, subject string) error {
	if strings.TrimSpace(subject) == "" {
		return errors.New("a subject is required")
	}

	return s.withTx(ctx, "target owner", func(queries *sqlcgen.Queries) error {
		if err := queries.EnsureTargetOwner(ctx, sqlcgen.EnsureTargetOwnerParams{
			Slot: subject, OwnerSubject: sql.NullString{String: subject, Valid: true},
			AuthorizationState: string(AuthorizationNotAuthorized), UpdatedAtUnix: time.Now().Unix(),
		}); err != nil {
			return fmt.Errorf("creating target owner: %w", err)
		}
		if err := queries.ClaimUnownedTarget(ctx, sqlcgen.ClaimUnownedTargetParams{
			OwnerSubject: sql.NullString{String: subject, Valid: true}, Slot: subject,
		}); err != nil {
			return fmt.Errorf("claiming target owner: %w", err)
		}
		return nil
	})
}

// ForEachTarget visits every target without exposing its Wahoo identity or
// refresh token. ownerSubject is empty for a target authorized before
// ownership existed and never since reassigned.
func (s *Store) ForEachTarget(ctx context.Context, visit func(id, authorizationState, ownerSubject string) error) error {
	if visit == nil {
		return errors.New("target visitor is required")
	}
	rows, err := s.queries.ListTargetStates(ctx)
	if err != nil {
		return fmt.Errorf("listing targets: %w", err)
	}
	for _, row := range rows {
		if err := visit(row.Slot, row.AuthorizationState, row.OwnerSubject); err != nil {
			return fmt.Errorf("visiting target: %w", err)
		}
	}

	return nil
}

// Target returns one target slot without exposing its refresh token.
func (s *Store) Target(ctx context.Context, targetID string) (Target, error) {
	row, err := s.queries.GetTarget(ctx, targetID)
	if errors.Is(err, sql.ErrNoRows) {
		return Target{}, ErrTargetNotFound
	}
	if err != nil {
		return Target{}, fmt.Errorf("reading target slot: %w", err)
	}

	return Target{
		ID: row.Slot, WahooUserID: row.WahooUserID, OwnerSubject: row.OwnerSubject,
		AuthorizationState: AuthorizationState(row.AuthorizationState),
	}, nil
}
