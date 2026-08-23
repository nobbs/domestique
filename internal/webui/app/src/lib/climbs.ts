/**
 * Finding the sustained climbs on a route: stretches worth naming to a rider
 * deciding where the effort is, rather than every metre steep enough to band.
 *
 * The gradient bands in `profile.ts` are unsigned — a descent bands exactly
 * as its mirror climb does, on purpose. A climb is the one place direction
 * matters, so this walks the same backward-windowed gradient a second time,
 * signed, and keeps only the runs that stay at or above the same edge that
 * already separates flat from not-flat everywhere else in the profile.
 */

import type { Position } from "../api/types";
import { cumulativeMetres, elevationOf, GRADIENT_BANDS, GRADIENT_WINDOW_METRES } from "./profile";

/** The gradient a run must hold to count as climbing, not merely uphill. */
const CLIMB_GRADIENT_PERCENT: number = GRADIENT_BANDS[0].limit;

/**
 * The shortest run of climbing ground reported on its own.
 *
 * The same floor `profile.ts` absorbs short band runs into: a climb shorter
 * than the window its own gradient was measured over was never sustained by
 * the definition that found it, and a short one still shows up folded into
 * whichever stretch of the ride surrounds it.
 */
const MIN_CLIMB_METRES = GRADIENT_WINDOW_METRES;

/** One sustained climb, as the ground it covers and what it costs to ride. */
export interface Climb {
  startMetres: number;
  endMetres: number;
  distanceMetres: number;
  /** Metres gained, counting only the rises within the climb. */
  ascentMetres: number;
  /** Net rise over the climb's length, which a dip inside it can only lower. */
  averageGradePercent: number;
  /** The steepest hundred metres inside the climb. */
  maxGradePercent: number;
}

interface Run {
  climbing: boolean;
  startIndex: number;
  endIndex: number;
}

/**
 * The route's sustained climbs, in the order they are ridden.
 *
 * Empty for geometry the profile itself refuses — missing elevation, or too
 * little of it to measure a rise over — so a partly surveyed route reports no
 * climbs rather than inventing one from absent data.
 */
export function findClimbs(coordinates: Position[]): Climb[] {
  const lastIndex = coordinates.length - 1;
  if (lastIndex < 1 || coordinates.some((point) => elevationOf(point) === undefined)) {
    return [];
  }
  const distances = cumulativeMetres(coordinates);
  if ((distances[lastIndex] ?? 0) <= 0) {
    return [];
  }
  const elevations = coordinates.map((point) => elevationOf(point) ?? 0);

  const { runs, gradients } = signedRuns(distances, elevations, lastIndex);

  return merged(runs, distances)
    .filter(
      (run) =>
        run.climbing &&
        (distances[run.endIndex + 1] ?? 0) - (distances[run.startIndex] ?? 0) >= MIN_CLIMB_METRES,
    )
    .map((run) => toClimb(run, distances, elevations, gradients));
}

/**
 * The route as runs of climbing and non-climbing ground, each segment
 * classified by the gradient measured back over `GRADIENT_WINDOW_METRES` —
 * the same look-back `profile.ts` bands with, signed instead of absolute.
 */
function signedRuns(
  distances: number[],
  elevations: number[],
  lastIndex: number,
): { runs: Run[]; gradients: number[] } {
  const gradients: number[] = [0];
  const runs: Run[] = [];
  let behind = 0;
  for (let index = 1; index <= lastIndex; index++) {
    while (
      behind + 1 < index &&
      (distances[index] ?? 0) - (distances[behind + 1] ?? 0) >= GRADIENT_WINDOW_METRES
    ) {
      behind++;
    }
    const run = (distances[index] ?? 0) - (distances[behind] ?? 0);
    const rise = (elevations[index] ?? 0) - (elevations[behind] ?? 0);
    const gradientPercent = run > 0 ? (rise / run) * 100 : 0;
    gradients.push(gradientPercent);
    const climbing = gradientPercent >= CLIMB_GRADIENT_PERCENT;

    const current = runs[runs.length - 1];
    if (current && current.climbing === climbing) {
      current.endIndex = index - 1;

      continue;
    }
    runs.push({ climbing, startIndex: index - 1, endIndex: index - 1 });
  }

  return { runs, gradients };
}

/** Absorbs runs too short to be sustained into the run before them. */
function merged(runs: Run[], distances: number[]): Run[] {
  const kept: Run[] = [];
  for (const run of runs) {
    const length = (distances[run.endIndex + 1] ?? 0) - (distances[run.startIndex] ?? 0);
    const previous = kept[kept.length - 1];
    if (previous && length < MIN_CLIMB_METRES) {
      previous.endIndex = run.endIndex;

      continue;
    }
    if (previous && previous.climbing === run.climbing) {
      previous.endIndex = run.endIndex;

      continue;
    }
    kept.push({ ...run });
  }

  return kept;
}

function toClimb(run: Run, distances: number[], elevations: number[], gradients: number[]): Climb {
  const startMetres = distances[run.startIndex] ?? 0;
  const endMetres = distances[run.endIndex + 1] ?? startMetres;
  const distanceMetres = endMetres - startMetres;
  const startElevation = elevations[run.startIndex] ?? 0;
  const endElevation = elevations[run.endIndex + 1] ?? startElevation;

  let ascentMetres = 0;
  for (let index = run.startIndex; index <= run.endIndex; index++) {
    const rise = (elevations[index + 1] ?? 0) - (elevations[index] ?? 0);
    if (rise > 0) {
      ascentMetres += rise;
    }
  }

  let maxGradePercent = 0;
  for (let index = run.startIndex + 1; index <= run.endIndex + 1; index++) {
    maxGradePercent = Math.max(maxGradePercent, gradients[index] ?? 0);
  }

  return {
    startMetres,
    endMetres,
    distanceMetres,
    ascentMetres,
    averageGradePercent:
      distanceMetres > 0 ? ((endElevation - startElevation) / distanceMetres) * 100 : 0,
    maxGradePercent,
  };
}
