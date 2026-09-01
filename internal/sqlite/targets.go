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

// Target is durable, non-secret state for one configured Wahoo target slot.
type Target struct {
	ID                 string
	WahooUserID        string
	AuthorizationState AuthorizationState
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

	queries := s.queries.WithTx(transaction)
	for _, targetID := range targetIDs {
		if err := queries.EnsureTarget(ctx, sqlcgen.EnsureTargetParams{
			Slot: targetID, AuthorizationState: string(AuthorizationNotAuthorized), UpdatedAtUnix: time.Now().Unix(),
		}); err != nil {
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
	rows, err := s.queries.ListTargets(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing target slots: %w", err)
	}
	targets := make([]Target, 0, len(rows))
	for _, row := range rows {
		targets = append(targets, Target{
			ID: row.Slot, WahooUserID: row.WahooUserID,
			AuthorizationState: AuthorizationState(row.AuthorizationState),
		})
	}

	return targets, nil
}

// ForEachTarget visits each configured target without exposing its Wahoo
// identity or refresh token.
func (s *Store) ForEachTarget(ctx context.Context, visit func(id, authorizationState string) error) error {
	if visit == nil {
		return errors.New("target visitor is required")
	}
	rows, err := s.queries.ListTargetStates(ctx)
	if err != nil {
		return fmt.Errorf("listing targets: %w", err)
	}
	for _, row := range rows {
		if err := visit(row.Slot, row.AuthorizationState); err != nil {
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
		ID: row.Slot, WahooUserID: row.WahooUserID,
		AuthorizationState: AuthorizationState(row.AuthorizationState),
	}, nil
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
