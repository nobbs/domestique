package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
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

// ForEachTarget visits each configured target without exposing its Wahoo
// identity or refresh token.
func (s *Store) ForEachTarget(ctx context.Context, visit func(id, authorizationState string) error) error {
	if visit == nil {
		return errors.New("target visitor is required")
	}
	rows, err := s.database.QueryContext(ctx, `
		SELECT slot, authorization_state FROM targets ORDER BY slot
	`)
	if err != nil {
		return fmt.Errorf("listing targets: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var id, authorizationState string
		if err := rows.Scan(&id, &authorizationState); err != nil {
			return fmt.Errorf("reading target: %w", err)
		}
		if err := visit(id, authorizationState); err != nil {
			return fmt.Errorf("visiting target: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating targets: %w", err)
	}

	return nil
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
