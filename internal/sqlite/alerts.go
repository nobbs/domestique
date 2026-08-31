package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"
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
	rows, err := s.database.QueryContext(ctx, `
		SELECT task, scope, alert, enabled FROM alert_toggle ORDER BY task, scope, alert
	`)
	if err != nil {
		return nil, fmt.Errorf("reading alert toggles: %w", err)
	}
	defer closeRows(rows)

	var toggles []AlertToggle
	for rows.Next() {
		var toggle AlertToggle
		if err := rows.Scan(&toggle.Task, &toggle.Scope, &toggle.Alert, &toggle.Enabled); err != nil {
			return nil, fmt.Errorf("reading an alert toggle: %w", err)
		}
		toggles = append(toggles, toggle)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading alert toggles: %w", err)
	}

	return toggles, nil
}

// SetAlertToggles records what an operator decided, replacing each decision
// whole. Deciding is what creates a row: an alert nobody has ruled on keeps
// whatever default its task carries.
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
	for _, toggle := range toggles {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO alert_toggle (task, scope, alert, enabled, updated_at_unix)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (task, scope, alert) DO UPDATE SET
				enabled = excluded.enabled,
				updated_at_unix = excluded.updated_at_unix
		`, toggle.Task, toggle.Scope, toggle.Alert, toggle.Enabled, now); err != nil {
			return fmt.Errorf("storing an alert toggle: %w", err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("committing alert toggles: %w", err)
	}

	return nil
}
