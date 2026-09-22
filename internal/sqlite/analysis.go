package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
)

// RecordAnalysisEnabled records now as the instant the analysis was enabled,
// unless an earlier start already did, and returns the instant that stands.
func (s *Store) RecordAnalysisEnabled(ctx context.Context, now time.Time) (time.Time, error) {
	since, err := s.queries.RecordAnalysisEnabled(ctx, now.Unix())
	if err != nil {
		return time.Time{}, fmt.Errorf("recording when the analysis was enabled: %w", err)
	}

	return time.Unix(since, 0).UTC(), nil
}

// ActivitiesAwaitingAnalysis lists the target's derived rides with no analysis
// that started at or after since, oldest first, at most limit of them. A head
// unit's ride of one of heldTypeIDs that ended at or after heldSince is left
// out; no types holds nothing.
func (s *Store) ActivitiesAwaitingAnalysis(
	ctx context.Context, targetID string, since, heldSince time.Time, heldTypeIDs []int, limit int,
) ([]activity.PendingAnalysis, error) {
	if limit <= 0 {
		return nil, errors.New("a positive limit is required")
	}
	rows, err := s.queries.ListActivitiesAwaitingAnalysis(ctx, sqlcgen.ListActivitiesAwaitingAnalysisParams{
		TargetSlot:       targetID,
		EnabledSinceUnix: since.Unix(),
		HeldSinceUnix:    heldSince.Unix(),
		HeldTypeIds:      typeIDList(heldTypeIDs),
		RowLimit:         int64(limit),
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

// StoreActivityAnalysis records what was said about one ride. The structured
// document is written for a revision 3 answer, and left NULL for an older one.
//
//nolint:gocritic // hugeParam: mirrors activity.AnalyseStore's own by-value signature.
func (s *Store) StoreActivityAnalysis(
	ctx context.Context, targetID string, id int64, analysis activity.Analysis,
) error {
	document, err := analysisDocumentColumn(analysis.PromptRevision, &analysis.Document)
	if err != nil {
		return err
	}
	if err := s.queries.UpsertActivityAnalysis(ctx, sqlcgen.UpsertActivityAnalysisParams{
		TargetSlot:     targetID,
		WorkoutID:      id,
		Text:           analysis.Text,
		Model:          analysis.Model,
		PromptRevision: int64(analysis.PromptRevision),
		AnalysedAtUnix: analysis.AnalysedAt.Unix(),
		Document:       document,
	}); err != nil {
		return fmt.Errorf("recording an activity analysis: %w", err)
	}

	return nil
}

// analysisDocumentColumn encodes a revision 3 document for storage, and NULL
// for anything earlier: an older revision's Document is always its zero value.
func analysisDocumentColumn(revision int, document *activity.AnalysisDocument) (sql.NullString, error) {
	if revision < 3 {
		return sql.NullString{}, nil
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("encoding an activity analysis document: %w", err)
	}

	return sql.NullString{String: string(encoded), Valid: true}, nil
}

// analysisDocumentOf decodes a stored document column, or the zero value for
// a row written before revision 3, which carries none.
func analysisDocumentOf(column sql.NullString) (activity.AnalysisDocument, error) {
	var document activity.AnalysisDocument
	if !column.Valid {
		return document, nil
	}
	if err := json.Unmarshal([]byte(column.String), &document); err != nil {
		return document, fmt.Errorf("decoding an activity analysis document: %w", err)
	}

	return document, nil
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
		document, err := analysisDocumentOf(row.Document)
		if err != nil {
			return nil, err
		}
		analyses = append(analyses, activity.Analysis{
			AnalysedAt:     time.Unix(row.AnalysedAtUnix, 0).UTC(),
			StartedAt:      time.Unix(row.StartedAtUnix, 0).UTC(),
			Text:           row.Text,
			Model:          row.Model,
			Document:       document,
			PromptRevision: int(row.PromptRevision),
		})
	}

	return analyses, nil
}

// ActivityAnalyses is what was said about each of one target's rides that
// started in [from, to), keyed by ride. A ride not analysed is absent from the map.
func (s *Store) ActivityAnalyses(
	ctx context.Context, targetID string, from, to time.Time,
) (map[int64]activity.Analysis, error) {
	rows, err := s.queries.ListActivityAnalyses(ctx, sqlcgen.ListActivityAnalysesParams{
		TargetSlot: targetID,
		FromUnix:   from.Truncate(time.Second).Unix(),
		ToUnix:     to.Truncate(time.Second).Unix(),
	})
	if err != nil {
		return nil, fmt.Errorf("reading the activity analyses: %w", err)
	}
	analyses := make(map[int64]activity.Analysis, len(rows))
	for _, row := range rows {
		document, err := analysisDocumentOf(row.Document)
		if err != nil {
			return nil, err
		}
		analyses[row.WorkoutID] = activity.Analysis{
			AnalysedAt:     time.Unix(row.AnalysedAtUnix, 0).UTC(),
			Text:           row.Text,
			Model:          row.Model,
			Document:       document,
			PromptRevision: int(row.PromptRevision),
		}
	}

	return analyses, nil
}

// ActivityStartedAt is when one of the target's rides started, and whether the
// target holds it at all.
func (s *Store) ActivityStartedAt(ctx context.Context, targetID string, id int64) (time.Time, bool, error) {
	started, err := s.queries.GetActivityStartedAt(ctx, sqlcgen.GetActivityStartedAtParams{TargetSlot: targetID, WorkoutID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("reading when an activity started: %w", err)
	}

	return time.Unix(started, 0).UTC(), true, nil
}
