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
// asked about, in one transaction. An empty set of hours is a read that found
// nothing: the record is still written, so the ride is not asked about again on
// every run for as long as the provider has nothing to say.
func (s *Store) StoreActivityWeather(
	ctx context.Context, targetID string, id int64, hours []activity.WeatherHour, readAt time.Time,
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
	for _, hour := range hours {
		if insertErr := queries.InsertActivityWeather(ctx, sqlcgen.InsertActivityWeatherParams{
			TargetSlot: targetID, WorkoutID: id, HourUnix: hour.Hour.Unix(),
			TemperatureCelsius:              hour.TemperatureCelsius,
			ApparentTemperatureCelsius:      hour.ApparentTemperatureCelsius,
			PrecipitationMillimetres:        hour.PrecipitationMillimetres,
			PrecipitationProbabilityPercent: nullFloat(hour.PrecipitationProbabilityPercent, hour.HasPrecipitationProbability),
			WindSpeedKmh:                    hour.WindSpeedKMH,
			WindDirectionDegrees:            hour.WindDirectionDegrees,
			WeatherCode:                     int64(hour.WeatherCode),
			CloudCoverPercent:               hour.CloudCoverPercent,
		}); insertErr != nil {
			return fmt.Errorf("recording an activity weather hour: %w", insertErr)
		}
	}
	if readErr := queries.RecordActivityWeatherRead(ctx, sqlcgen.RecordActivityWeatherReadParams{
		TargetSlot: targetID, WorkoutID: id, ReadAtUnix: readAt.Unix(), Hours: int64(len(hours)),
	}); readErr != nil {
		return fmt.Errorf("recording an activity weather read: %w", readErr)
	}
	if commitErr := transaction.Commit(); commitErr != nil {
		return fmt.Errorf("committing the activity weather: %w", commitErr)
	}

	return nil
}

// ActivityWeather is every hour one target holds, keyed by ride. One read for
// the whole target rather than one per ride: the listing card wants a line
// about each of them at once.
func (s *Store) ActivityWeather(ctx context.Context, targetID string) (map[int64][]activity.WeatherHour, error) {
	rows, err := s.queries.ListActivityWeather(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("reading the activity weather: %w", err)
	}
	weather := map[int64][]activity.WeatherHour{}
	for index := range rows {
		row := &rows[index]
		weather[row.WorkoutID] = append(weather[row.WorkoutID], activity.WeatherHour{
			Hour:                            time.Unix(row.HourUnix, 0).UTC(),
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

	return weather, nil
}

// ActivityWeatherHours is one ride's weather, in order. Read on its own rather
// than out of the whole target's: a ride page asks about one ride.
func (s *Store) ActivityWeatherHours(
	ctx context.Context, targetID string, id int64,
) ([]activity.WeatherHour, error) {
	rows, err := s.queries.ListActivityWeatherHours(ctx, sqlcgen.ListActivityWeatherHoursParams{
		TargetSlot: targetID, WorkoutID: id,
	})
	if err != nil {
		return nil, fmt.Errorf("reading the activity weather: %w", err)
	}
	hours := make([]activity.WeatherHour, 0, len(rows))
	for index := range rows {
		row := &rows[index]
		hours = append(hours, activity.WeatherHour{
			Hour:                            time.Unix(row.HourUnix, 0).UTC(),
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

	return hours, nil
}
