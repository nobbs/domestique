package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// TaskSchedule reports which tasks an operator has switched off. A task absent
// from the result is undecided, which runs: a new task reaches the schedule
// without anybody turning it on.
func (s *Store) TaskSchedule(ctx context.Context) (map[string]bool, error) {
	rows, err := s.database.QueryContext(ctx, `SELECT task, enabled FROM task_schedule`)
	if err != nil {
		return nil, fmt.Errorf("reading the task schedule: %w", err)
	}
	defer closeRows(rows)

	schedule := make(map[string]bool)
	for rows.Next() {
		var (
			task    string
			enabled bool
		)
		if err := rows.Scan(&task, &enabled); err != nil {
			return nil, fmt.Errorf("reading the task schedule: %w", err)
		}
		schedule[task] = enabled
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the task schedule: %w", err)
	}

	return schedule, nil
}

// SetTaskSchedule records whether the schedule may start one task.
func (s *Store) SetTaskSchedule(ctx context.Context, task string, enabled bool) error {
	if task == "" {
		return errors.New("task is required")
	}
	if _, err := s.database.ExecContext(ctx, `
		INSERT INTO task_schedule (task, enabled, updated_at_unix) VALUES (?, ?, ?)
		ON CONFLICT(task) DO UPDATE SET enabled = excluded.enabled, updated_at_unix = excluded.updated_at_unix
	`, task, enabled, time.Now().Unix()); err != nil {
		return fmt.Errorf("recording the task schedule: %w", err)
	}

	return nil
}
