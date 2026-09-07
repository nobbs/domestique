package sqlite

import (
	"context"

	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
)

// ActivitiesAwaitingWeather lists the target's rides nobody has asked the
// weather about, newest first, at most limit of them. A ride whose read failed
// for good left a record saying so and is not among them: asking again spends a
// request to be told the same thing.
func (s *Store) ActivitiesAwaitingWeather(
	ctx context.Context, targetID string, limit int,
) ([]activity.PendingWeather, error) {
	rows, err := s.queries.ListActivitiesAwaitingWeather(ctx, sqlcgen.ListActivitiesAwaitingWeatherParams{
		TargetSlot: targetID,
		RowLimit:   int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("listing activities awaiting weather: %w", err)
	}
	pending := make([]activity.PendingWeather, 0, len(rows))
	for _, row := range rows {
		pending = append(pending, activity.PendingWeather{
			StartedAt:      time.Unix(row.StartedAtUnix, 0).UTC(),
			ID:             row.WorkoutID,
			ElapsedSeconds: row.ElapsedSeconds,
		})
	}

	return pending, nil
}

// StoreActivityWeather replaces one ride's weather and records that it was
// asked about, in one transaction. An empty set of steps is a read that found
// nothing: the record is still written, so the ride is not asked about again on
// every run for as long as the provider has nothing to say.
func (s *Store) StoreActivityWeather(
	ctx context.Context, targetID string, id int64, steps []activity.WeatherStep, readAt time.Time,
) error {
	transaction, beginErr := s.database.BeginTx(ctx, nil)
	if beginErr != nil {
		return fmt.Errorf("starting the activity weather write: %w", beginErr)
	}
	defer rollback(transaction)
	queries := s.queries.WithTx(transaction)
	if deleteErr := queries.DeleteActivityWeather(ctx, sqlcgen.DeleteActivityWeatherParams{
		TargetSlot: targetID, WorkoutID: id,
	}); deleteErr != nil {
		return fmt.Errorf("clearing prior activity weather: %w", deleteErr)
	}
	for _, step := range steps {
		if insertErr := queries.InsertActivityWeather(ctx, sqlcgen.InsertActivityWeatherParams{
			TargetSlot: targetID, WorkoutID: id, HourUnix: step.At.Unix(), StepSeconds: int64(step.Step.Seconds()),
			TemperatureCelsius:              step.TemperatureCelsius,
			ApparentTemperatureCelsius:      step.ApparentTemperatureCelsius,
			PrecipitationMillimetres:        step.PrecipitationMillimetres,
			PrecipitationProbabilityPercent: nullFloat(step.PrecipitationProbabilityPercent, step.HasPrecipitationProbability),
			WindSpeedKmh:                    step.WindSpeedKMH,
			WindDirectionDegrees:            step.WindDirectionDegrees,
			WeatherCode:                     int64(step.WeatherCode),
			CloudCoverPercent:               step.CloudCoverPercent,
		}); insertErr != nil {
			return fmt.Errorf("recording an activity weather step: %w", insertErr)
		}
	}
	// Hours counts the rows written, which are steps and no longer always hours.
	// The column keeps its name so the previous release can still read it.
	if readErr := queries.RecordActivityWeatherRead(ctx, sqlcgen.RecordActivityWeatherReadParams{
		TargetSlot: targetID, WorkoutID: id, ReadAtUnix: readAt.Unix(), Hours: int64(len(steps)),
	}); readErr != nil {
		return fmt.Errorf("recording an activity weather read: %w", readErr)
	}
	if commitErr := transaction.Commit(); commitErr != nil {
		return fmt.Errorf("committing the activity weather: %w", commitErr)
	}

	return nil
}

// ActivityWeatherSummaries is what each of one target's rides came to, keyed by
// ride. Summed in SQL rather than in Go: a rider with years of history holds a
// step of weather for every step they have ridden, and the listing wants one
// line about each ride rather than all of them.
func (s *Store) ActivityWeatherSummaries(
	ctx context.Context, targetID string,
) (map[int64]activity.WeatherSummary, error) {
	rows, err := s.queries.SummariseActivityWeather(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("reading the activity weather: %w", err)
	}
	summaries := make(map[int64]activity.WeatherSummary, len(rows))
	for _, row := range rows {
		summaries[row.WorkoutID] = activity.WeatherSummary{
			TemperatureMinCelsius:    row.TemperatureMinCelsius,
			TemperatureMaxCelsius:    row.TemperatureMaxCelsius,
			WindSpeedKMH:             row.WindSpeedKmh,
			PrecipitationMillimetres: row.PrecipitationMillimetres,
			WeatherCode:              int(row.WeatherCode),
		}
	}

	return summaries, nil
}

// ActivityWeatherSteps is one ride's weather, in order. Read on its own rather
// than out of the whole target's: a ride page asks about one ride.
func (s *Store) ActivityWeatherSteps(
	ctx context.Context, targetID string, id int64,
) ([]activity.WeatherStep, error) {
	rows, err := s.queries.ListActivityWeatherSteps(ctx, sqlcgen.ListActivityWeatherStepsParams{
		TargetSlot: targetID, WorkoutID: id,
	})
	if err != nil {
		return nil, fmt.Errorf("reading the activity weather: %w", err)
	}
	steps := make([]activity.WeatherStep, 0, len(rows))
	for index := range rows {
		row := &rows[index]
		steps = append(steps, activity.WeatherStep{
			At:                              time.Unix(row.AtUnix, 0).UTC(),
			Step:                            time.Duration(row.StepSeconds) * time.Second,
			TemperatureCelsius:              row.TemperatureCelsius,
			ApparentTemperatureCelsius:      row.ApparentTemperatureCelsius,
			PrecipitationMillimetres:        row.PrecipitationMillimetres,
			PrecipitationProbabilityPercent: row.PrecipitationProbabilityPercent.Float64,
			WindSpeedKMH:                    row.WindSpeedKmh,
			WindDirectionDegrees:            row.WindDirectionDegrees,
			CloudCoverPercent:               row.CloudCoverPercent,
			WeatherCode:                     int(row.WeatherCode),
			HasPrecipitationProbability:     row.PrecipitationProbabilityPercent.Valid,
		})
	}

	return steps, nil
}
