/**
 * Turns a stage's predicted moving time into the short list of places and
 * moments worth asking the weather about.
 *
 * A rider does not want a forecast at every recorded point — that is far more
 * requests than any weather API budgets for, and far more detail than a rider
 * can act on. Samples are spaced evenly across the *time* the ride takes
 * rather than the distance it covers, because weather changes with the clock,
 * not the odometer: a slow climb deserves closer attention than a fast
 * descent covering the same ground in a third of the time.
 *
 * Distance and position reuse `cumulativeMetres` and `haversineMetres` from
 * `profile.ts` — the same measurement the elevation profile plots against —
 * so a forecast sample never lands somewhere the profile disagrees a climb is.
 *
 * Pure: no DOM, no fetching, no rendering. What a page does with the samples —
 * request a forecast, draw a marker — is issue #208's concern, not this one.
 */

import type { Position } from "../api/types";
import { cumulativeMetres } from "./profile";

/**
 * Two samples per forecast hour.
 *
 * The calibration knob: enough resolution to show a weather front arriving
 * mid-ride without requesting a forecast at every recorded point. It trades
 * sample count against how sharply a change in conditions shows up.
 */
export const SAMPLE_INTERVAL_SECONDS = 1800;

/**
 * No two samples closer than five kilometres.
 *
 * Thirty minutes of a steep climb can be four kilometres, which sits inside
 * one ICON-D2 grid cell — four samples there would just return one number
 * four times.
 */
export const MIN_SAMPLE_SPACING_METRES = 5000;

/**
 * Matches the weather endpoint's own per-request cap (`maximumWeatherPoints`
 * in `internal/httpapi/routes_weather.go`). When this binds, the interval
 * between samples widens; the route itself is never truncated.
 */
export const MAX_SAMPLES = 48;

/** One place and moment along a stage worth requesting a forecast for. */
export interface ForecastSample {
  position: Position;
  distanceMetres: number;
  arrivalAt: Date;
}

/**
 * Picks the samples worth a forecast request: evenly spaced across the ride's
 * moving time, then thinned so no two sit closer than
 * `MIN_SAMPLE_SPACING_METRES` apart on the ground.
 *
 * `cumulativeSeconds` is the predicted moving time at each coordinate, as the
 * API attaches it to a stage's geometry — indexed 1:1 with `coordinates` and
 * assumed non-decreasing. It is optional for the same
 * reason that field is: a stage nothing has predicted a time for has no clock
 * to sample against, and this never guesses one. It returns `[]` for an
 * absent or empty series, mismatched lengths, or a total moving time of
 * zero, leaving the caller to decide what an unpredicted stage shows.
 */
export function forecastSamples(
  coordinates: Position[],
  cumulativeSeconds: number[] | undefined,
  startAt: Date,
): ForecastSample[] {
  if (
    cumulativeSeconds === undefined ||
    coordinates.length === 0 ||
    cumulativeSeconds.length === 0 ||
    coordinates.length !== cumulativeSeconds.length
  ) {
    return [];
  }

  const total = cumulativeSeconds[cumulativeSeconds.length - 1] ?? 0;
  if (total <= 0) {
    return [];
  }

  const slots = Math.min(MAX_SAMPLES, Math.max(2, Math.floor(total / SAMPLE_INTERVAL_SECONDS) + 1));

  // One forward pass: the target moving time rises monotonically with the
  // slot, and so does the first coordinate index that reaches it, so the same
  // cursor serves every slot rather than rescanning from the start each time.
  const candidates: number[] = [];
  let cursor = 0;
  for (let slot = 0; slot < slots; slot++) {
    const target = (total * slot) / (slots - 1);
    while (cursor < cumulativeSeconds.length - 1 && (cumulativeSeconds[cursor] ?? 0) < target) {
      cursor++;
    }
    if (candidates[candidates.length - 1] !== cursor) {
      candidates.push(cursor);
    }
  }

  // A ride whose last coordinates share one moving time — a stage ending in a
  // zero-length segment — leaves the cursor on the first of them, so the finish
  // itself would never be sampled. It is the point a rider most wants, so it
  // replaces the sample before it once the cap is already full.
  const lastIndex = coordinates.length - 1;
  if (candidates[candidates.length - 1] !== lastIndex) {
    if (candidates.length >= MAX_SAMPLES) {
      candidates.pop();
    }
    candidates.push(lastIndex);
  }

  const distances = cumulativeMetres(coordinates);
  const kept: number[] = [];
  candidates.forEach((coordinateIndex, position) => {
    const isLast = position === candidates.length - 1;
    const previousKept = kept[kept.length - 1];
    if (previousKept === undefined) {
      kept.push(coordinateIndex);
      return;
    }
    const gap = (distances[coordinateIndex] ?? 0) - (distances[previousKept] ?? 0);
    if (gap >= MIN_SAMPLE_SPACING_METRES) {
      kept.push(coordinateIndex);
    } else if (isLast) {
      // Never drop the last sample: drop its too-close predecessor instead,
      // unless that predecessor is itself the first sample, which never drops.
      if (kept.length > 1) {
        kept.pop();
      }
      kept.push(coordinateIndex);
    }
  });

  return kept.map((coordinateIndex) => ({
    position: coordinates[coordinateIndex] as Position,
    distanceMetres: distances[coordinateIndex] ?? 0,
    arrivalAt: new Date(startAt.getTime() + (cumulativeSeconds[coordinateIndex] ?? 0) * 1000),
  }));
}
