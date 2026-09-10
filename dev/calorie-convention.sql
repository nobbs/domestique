-- #623: does the estimate sit low, or do the head unit's calories sit high?
-- Indoor rides carry measured power, so the checks the outdoor estimate fails
-- can be re-run against a figure that is not an estimate at all.
--
-- Aggregate only: no ride identifier, date, position or altitude value. Safe to
-- paste into an issue.
--
--   sqlite3 -header -column .local/dev/state.db < dev/calorie-convention.sql
--
-- Indoor workout types are wahoo.IndoorWorkoutTypes(); anything else is ridden
-- over real ground and carries no meter.

.print '== 1. energy vs device calories (handover §10), by where the power came from =='
WITH ride AS (
  SELECT
    CASE WHEN a.workout_type_id IN (12, 49, 61, 68) THEN 'indoor' ELSE 'outdoor' END AS place,
    CASE WHEN s.average_power_watts > 0 THEN 'measured' ELSE 'estimated' END        AS source,
    COALESCE(NULLIF(s.average_power_watts, 0), m.estimated_power_watts)
      * s.timer_seconds / 1000.0                                                   AS kj,
    s.calories_kcal                                                                AS kcal
  FROM activities AS a
  JOIN activity_session AS s USING (target_slot, workout_id)
  LEFT JOIN activity_metrics AS m USING (target_slot, workout_id)
  WHERE s.calories_kcal > 0 AND s.timer_seconds > 0
),
scored AS (
  SELECT place, source, kj / 4.184 / 0.22 / kcal - 1 AS energy_error, kj / (kcal * 4.184) AS efficiency
  FROM ride WHERE kj > 0
),
ranked AS (
  SELECT *,
    ROW_NUMBER() OVER (PARTITION BY place, source ORDER BY energy_error) AS rank_error,
    ROW_NUMBER() OVER (PARTITION BY place, source ORDER BY efficiency)   AS rank_efficiency,
    COUNT(*)     OVER (PARTITION BY place, source)                       AS n
  FROM scored
)
SELECT
  place, source, MAX(n) AS rides,
  ROUND(AVG(CASE WHEN rank_error      IN ((n + 1) / 2, (n + 2) / 2) THEN energy_error END) * 100, 1) AS median_energy_error_pct,
  ROUND(AVG(CASE WHEN rank_efficiency IN ((n + 1) / 2, (n + 2) / 2) THEN efficiency   END), 3)       AS median_implied_efficiency
FROM ranked
GROUP BY place, source
ORDER BY place, source;

.print ''
.print '== 2. power at matched heart rate: the rider is their own calibration ride =='
.print '   (indoor heart rate runs high for a given power, so this understates the gap)'
WITH ride AS (
  SELECT
    CASE WHEN a.workout_type_id IN (12, 49, 61, 68) THEN 'indoor' ELSE 'outdoor' END AS place,
    s.average_heart_rate_bpm                                           AS hr,
    COALESCE(NULLIF(s.average_power_watts, 0), m.estimated_power_watts) AS watts
  FROM activities AS a
  JOIN activity_session AS s USING (target_slot, workout_id)
  LEFT JOIN activity_metrics AS m USING (target_slot, workout_id)
  WHERE s.average_heart_rate_bpm > 0 AND s.timer_seconds > 600
)
SELECT CAST(hr / 10 AS INT) * 10 AS hr_bin, place, COUNT(*) AS rides, ROUND(AVG(watts), 1) AS mean_watts
FROM ride WHERE watts > 0
GROUP BY hr_bin, place
ORDER BY hr_bin, place;

.print ''
.print '== 3. the confound: outdoor rides coast, indoor rides do not =='
WITH r AS (
  SELECT CASE WHEN a.workout_type_id IN (12, 49, 61, 68) THEN 'indoor' ELSE 'outdoor' END AS place,
         c.cadence_rpm
  FROM activities AS a
  JOIN activity_records AS c USING (target_slot, workout_id)
  WHERE c.cadence_rpm IS NOT NULL
)
SELECT place, COUNT(*) AS samples, ROUND(100.0 * SUM(cadence_rpm = 0) / COUNT(*), 1) AS pct_zero_cadence
FROM r GROUP BY place;
