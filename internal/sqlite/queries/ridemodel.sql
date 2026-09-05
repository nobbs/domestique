-- name: GetRideModelCoefficients :one
SELECT seconds_per_km, seconds_per_ascent_m, calibration_cutoff_unix, evaluated_rides,
  bias_percent, mae_percent, p90_percent, training_window_months
FROM ridemodel_coefficients
WHERE id = 1;

-- name: UpsertRideModelCoefficients :exec
INSERT INTO ridemodel_coefficients (id, seconds_per_km, seconds_per_ascent_m, calibration_cutoff_unix,
  evaluated_rides, bias_percent, mae_percent, p90_percent, training_window_months, updated_at_unix)
VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET seconds_per_km = excluded.seconds_per_km,
  seconds_per_ascent_m = excluded.seconds_per_ascent_m,
  calibration_cutoff_unix = excluded.calibration_cutoff_unix,
  evaluated_rides = excluded.evaluated_rides, bias_percent = excluded.bias_percent,
  mae_percent = excluded.mae_percent, p90_percent = excluded.p90_percent,
  training_window_months = excluded.training_window_months,
  updated_at_unix = excluded.updated_at_unix;
