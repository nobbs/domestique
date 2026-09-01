package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
)

// TaskSchedule reports which tasks an operator has switched off. A task absent
// from the result is undecided, which runs: a new task reaches the schedule
// without anybody turning it on.
func (s *Store) TaskSchedule(ctx context.Context) (map[string]bool, error) {
	rows, err := s.queries.ListTaskSchedule(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading the task schedule: %w", err)
	}
	schedule := make(map[string]bool)
	for _, row := range rows {
		schedule[row.Task] = row.Enabled != 0
	}

	return schedule, nil
}

// SetTaskSchedule records whether the schedule may start one task.
func (s *Store) SetTaskSchedule(ctx context.Context, task string, enabled bool) error {
	if task == "" {
		return errors.New("task is required")
	}
	if err := s.queries.UpsertTaskSchedule(ctx, sqlcgen.UpsertTaskScheduleParams{
		Task: task, Enabled: boolInteger(enabled), UpdatedAtUnix: time.Now().Unix(),
	}); err != nil {
		return fmt.Errorf("recording the task schedule: %w", err)
	}

	return nil
}
