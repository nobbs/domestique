package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
)

// RecordAnalysisEnabled records now as the instant the analysis was enabled,
// unless an earlier start already did, and returns the instant that stands.
func (s *Store) RecordAnalysisEnabled(ctx context.Context, now time.Time) (time.Time, error) {
	if err := s.queries.RecordAnalysisEnabled(ctx, now.Unix()); err != nil {
		return time.Time{}, fmt.Errorf("recording when the analysis was enabled: %w", err)
	}
	since, err := s.queries.GetAnalysisEnabledSince(ctx)
	if err != nil {
		return time.Time{}, fmt.Errorf("reading when the analysis was enabled: %w", err)
	}

	return time.Unix(since, 0).UTC(), nil
}

// ActivitiesAwaitingAnalysis lists the target's derived rides with no analysis
// that started at or after since, oldest first, at most limit of them.
func (s *Store) ActivitiesAwaitingAnalysis(
	ctx context.Context, targetID string, since time.Time, limit int,
) ([]activity.PendingAnalysis, error) {
	if limit <= 0 {
		return nil, errors.New("a positive limit is required")
	}
	rows, err := s.queries.ListActivitiesAwaitingAnalysis(ctx, sqlcgen.ListActivitiesAwaitingAnalysisParams{
		TargetSlot: targetID, EnabledSinceUnix: since.Unix(), RowLimit: int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("listing activities awaiting analysis: %w", err)
	}
	pending := make([]activity.PendingAnalysis, 0, len(rows))
	for _, row := range rows {
		pending = append(pending, activity.PendingAnalysis{
			StartedAt: time.Unix(row.StartedAtUnix, 0).UTC(),
			ID:        row.WorkoutID,
		})
	}

	return pending, nil
}

// StoreActivityAnalysis records what was said about one ride.
func (s *Store) StoreActivityAnalysis(
	ctx context.Context, targetID string, id int64, analysis activity.Analysis,
) error {
	if err := s.queries.UpsertActivityAnalysis(ctx, sqlcgen.UpsertActivityAnalysisParams{
		TargetSlot:     targetID,
		WorkoutID:      id,
		Text:           analysis.Text,
		Model:          analysis.Model,
		PromptRevision: int64(analysis.PromptRevision),
		AnalysedAtUnix: analysis.AnalysedAt.Unix(),
	}); err != nil {
		return fmt.Errorf("recording an activity analysis: %w", err)
	}

	return nil
}

// AnalysesBefore is what was said about the target's rides that started before
// one instant, newest first, at most limit of them.
func (s *Store) AnalysesBefore(
	ctx context.Context, targetID string, before time.Time, limit int,
) ([]activity.Analysis, error) {
	if limit <= 0 {
		return nil, errors.New("a positive limit is required")
	}
	rows, err := s.queries.ListAnalysesBefore(ctx, sqlcgen.ListAnalysesBeforeParams{
		TargetSlot: targetID, StartedBeforeUnix: before.Unix(), RowLimit: int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("reading earlier activity analyses: %w", err)
	}
	analyses := make([]activity.Analysis, 0, len(rows))
	for _, row := range rows {
		analyses = append(analyses, activity.Analysis{
			AnalysedAt:     time.Unix(row.AnalysedAtUnix, 0).UTC(),
			Text:           row.Text,
			Model:          row.Model,
			PromptRevision: int(row.PromptRevision),
		})
	}

	return analyses, nil
}
