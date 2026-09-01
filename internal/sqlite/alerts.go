package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
)

// AlertToggle is one operator decision: whether one alert of one task, over one
// scope, is delivered.
type AlertToggle struct {
	Task    string
	Scope   string
	Alert   string
	Enabled bool
}

// AlertToggles reads every decision an operator has made. What is absent is
// what nobody has decided, which is not the same as switched off: the caller
// applies its own default to those.
func (s *Store) AlertToggles(ctx context.Context) ([]AlertToggle, error) {
	rows, err := s.queries.ListAlertToggles(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading alert toggles: %w", err)
	}
	toggles := make([]AlertToggle, 0, len(rows))
	for _, row := range rows {
		toggles = append(toggles, AlertToggle{
			Task: row.Task, Scope: row.Scope, Alert: row.Alert, Enabled: row.Enabled != 0,
		})
	}

	return toggles, nil
}

// SetAlertToggles records what an operator decided, replacing each decision
// whole.
func (s *Store) SetAlertToggles(ctx context.Context, toggles []AlertToggle) error {
	for _, toggle := range toggles {
		if toggle.Task == "" || toggle.Alert == "" {
			return errors.New("an alert toggle needs a task and an alert")
		}
	}

	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("storing alert toggles: %w", err)
	}
	defer rollback(transaction)

	now := time.Now().UTC().Unix()
	queries := s.queries.WithTx(transaction)
	for _, toggle := range toggles {
		if err := queries.UpsertAlertToggle(ctx, sqlcgen.UpsertAlertToggleParams{
			Task: toggle.Task, Scope: toggle.Scope, Alert: toggle.Alert,
			Enabled: boolInteger(toggle.Enabled), UpdatedAtUnix: now,
		}); err != nil {
			return fmt.Errorf("storing an alert toggle: %w", err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("committing alert toggles: %w", err)
	}

	return nil
}
