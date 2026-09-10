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

.print ''
.print '== 4. what the model omits: aero convexity lost to the speed window =='
.print '   (the estimate feeds window-mean speed into a v^3 term; raw vs 16 s vs 49 s)'
WITH s AS (
  SELECT c.target_slot AS ts, c.workout_id AS wid, c.record_index AS i,
         c.recorded_at_unix AS t, c.distance_metres AS d
  FROM activities AS a
  JOIN activity_records AS c USING (target_slot, workout_id)
  WHERE a.workout_type_id NOT IN (12, 49, 61, 68) AND c.distance_metres IS NOT NULL
),
v AS (
  SELECT ts, wid, i, (d - LAG(d) OVER w) / NULLIF(t - LAG(t) OVER w, 0) AS v
  FROM s WINDOW w AS (PARTITION BY ts, wid ORDER BY i)
),
f AS (SELECT * FROM v WHERE v IS NOT NULL AND v >= 0 AND v < 25),
smoothed AS (
  SELECT v,
    AVG(v) OVER (PARTITION BY ts, wid ORDER BY i ROWS BETWEEN  8 PRECEDING AND  8 FOLLOWING) AS v16,
    AVG(v) OVER (PARTITION BY ts, wid ORDER BY i ROWS BETWEEN 24 PRECEDING AND 24 FOLLOWING) AS v49
  FROM f
)
SELECT COUNT(*) AS samples,
       ROUND(AVG(v * v * v) / AVG(v16 * v16 * v16), 3) AS aero_loss_16s,
       ROUND(AVG(v * v * v) / AVG(v49 * v49 * v49), 3) AS aero_loss_49s
FROM smoothed;

.print ''
.print '== 5. what the model omits: the inertia term under the zero clamp =='
.print '   (net over a ride is nil, but the clamp keeps the accelerations and'
.print '    refunds none of the braking, which is what a rider actually pays)'
WITH s AS (
  SELECT c.target_slot AS ts, c.workout_id AS wid, c.record_index AS i,
         c.recorded_at_unix AS t, c.distance_metres AS d
  FROM activities AS a
  JOIN activity_records AS c USING (target_slot, workout_id)
  WHERE a.workout_type_id NOT IN (12, 49, 61, 68) AND c.distance_metres IS NOT NULL
),
v AS (
  SELECT ts, wid, i, t, (d - LAG(d) OVER w) / NULLIF(t - LAG(t) OVER w, 0) AS v
  FROM s WINDOW w AS (PARTITION BY ts, wid ORDER BY i)
),
f AS (SELECT * FROM v WHERE v IS NOT NULL AND v >= 0 AND v < 25),
smoothed AS (
  SELECT ts, wid, i, t,
         AVG(v) OVER (PARTITION BY ts, wid ORDER BY i ROWS BETWEEN 5 PRECEDING AND 5 FOLLOWING) AS v
  FROM f
),
acc AS (
  SELECT v, (v - LAG(v) OVER w) / NULLIF(t - LAG(t) OVER w, 0) AS a
  FROM smoothed WINDOW w AS (PARTITION BY ts, wid ORDER BY i)
)
-- 93.5 kg is the rider profile's 92 kg total plus the handover's 1.5 kg of
-- equivalent linear mass for wheel rotational inertia.
SELECT COUNT(*) AS samples,
       ROUND(AVG(93.5 * a * v), 2)                          AS mean_inertia_w_net,
       ROUND(AVG(MAX(93.5 * a * v, 0)), 1)                  AS mean_inertia_w_positive,
       ROUND(AVG(MAX(93.5 * a * v, 0)) - AVG(93.5 * a * v), 1) AS clamp_asymmetry_w
FROM acc WHERE a IS NOT NULL AND ABS(a) < 5;
