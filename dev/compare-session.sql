-- One row per ride with the figure three sources gave it: the FIT session
-- (activity_session), the derivation over the samples (activity_metrics) and
-- Wahoo's summary (activities.raw_summary_json), largest relative gap first.
--
--   sqlite3 -header -column .local/dev/state.db < dev/compare-session.sql
WITH per_ride AS (
  SELECT
    a.target_slot, a.workout_id, datetime(a.started_at_unix, 'unixepoch') AS started,
    s.max_speed_kmh                                     AS max_speed_session,
    m.max_speed_kmh                                     AS max_speed_derived,
    s.average_heart_rate_bpm                            AS avg_hr_session,
    m.average_heart_rate_bpm                            AS avg_hr_derived,
    CAST(json_extract(a.raw_summary_json, '$.heart_rate_avg') AS REAL) AS avg_hr_api,
    s.max_heart_rate_bpm                                AS max_hr_session,
    m.max_heart_rate_bpm                                AS max_hr_derived,
    s.average_cadence_rpm                               AS avg_cadence_session,
    m.average_cadence_rpm                               AS avg_cadence_derived,
    CAST(json_extract(a.raw_summary_json, '$.cadence_avg') AS REAL) AS avg_cadence_api,
    s.average_power_watts                               AS avg_power_session,
    m.average_power_watts                               AS avg_power_derived,
    CAST(json_extract(a.raw_summary_json, '$.power_avg') AS REAL) AS avg_power_api,
    s.distance_metres                                   AS distance_session,
    CAST(json_extract(a.raw_summary_json, '$.distance_accum') AS REAL) AS distance_api,
    s.timer_seconds                                     AS moving_session,
    CAST(json_extract(a.raw_summary_json, '$.duration_active_accum') AS REAL) AS moving_api,
    s.elapsed_seconds                                   AS elapsed_session,
    CAST(json_extract(a.raw_summary_json, '$.duration_total_accum') AS REAL) AS elapsed_api,
    s.ascent_metres                                     AS ascent_session,
    CAST(json_extract(a.raw_summary_json, '$.ascent_accum') AS REAL) AS ascent_api
  FROM activities AS a
  JOIN activity_session AS s ON s.target_slot = a.target_slot AND s.workout_id = a.workout_id
  LEFT JOIN activity_metrics AS m ON m.target_slot = a.target_slot AND m.workout_id = a.workout_id
)
SELECT *,
  MAX(
    COALESCE(ABS(max_speed_derived - max_speed_session) / NULLIF(max_speed_session, 0), 0),
    COALESCE(ABS(avg_hr_derived - avg_hr_session) / NULLIF(avg_hr_session, 0), 0),
    COALESCE(ABS(max_hr_derived - max_hr_session) / NULLIF(max_hr_session, 0), 0),
    COALESCE(ABS(avg_cadence_derived - avg_cadence_session) / NULLIF(avg_cadence_session, 0), 0),
    COALESCE(ABS(avg_power_derived - avg_power_session) / NULLIF(avg_power_session, 0), 0),
    COALESCE(ABS(distance_api - distance_session) / NULLIF(distance_session, 0), 0),
    COALESCE(ABS(moving_api - moving_session) / NULLIF(moving_session, 0), 0),
    COALESCE(ABS(elapsed_api - elapsed_session) / NULLIF(elapsed_session, 0), 0),
    COALESCE(ABS(ascent_api - ascent_session) / NULLIF(ascent_session, 0), 0)
  ) AS worst_relative_gap
FROM per_ride
ORDER BY worst_relative_gap DESC, started DESC;
