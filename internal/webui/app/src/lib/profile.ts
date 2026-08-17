/**
 * Turns route geometry into an elevation profile: elevation as a function of
 * distance along the route.
 *
 * Distance is measured here rather than taken from the API because the profile
 * needs it per point, not per route. It uses the same spherical model the
 * service does, so the axis agrees with the distance shown beside it.
 *
 * The samples are spaced evenly by distance, not by point index. Source points
 * are not evenly spaced, so plotting them by index would stretch dense sections
 * and compress sparse ones — the profile would misreport where a climb is.
 */

import type { Position } from "../api/types";

const EARTH_RADIUS_METRES = 6_371_000;

/**
 * Gradient classes, gentlest first.
 *
 * Steepness is an ordered measure, so it gets an ordinal ramp — one hue running
 * light to dark — rather than the green-amber-red heat scale cycling apps
 * usually reach for. That scale fails as a chart ramp: its lightness is not
 * monotone (the amber is lighter than the green, so severity stops reading as
 * "more"), and the amber drops to 1.79:1 against a light surface.
 *
 * The bands use absolute steepness, so a fast descent is marked as clearly as
 * the climb it mirrors.
 */
export const GRADIENT_BANDS = [
  { limit: 3, label: "< 3%" },
  { limit: 6, label: "3–6%" },
  { limit: 9, label: "6–9%" },
  { limit: Number.POSITIVE_INFINITY, label: "≥ 9%" },
] as const;

/** The shortest span a gradient is measured over, matching the service. */
const GRADIENT_WINDOW_METRES = 100;

export function gradientBand(percent: number): number {
  const magnitude = Math.abs(percent);

  return GRADIENT_BANDS.findIndex((band) => magnitude < band.limit);
}

export interface ProfileSample {
  distanceMetres: number;
  elevationMetres: number;
  gradientPercent: number;
  band: number;
}

export interface Profile {
  samples: ProfileSample[];
  totalDistanceMetres: number;
  minElevationMetres: number;
  maxElevationMetres: number;
}

function haversineMetres(from: Position, to: Position): number {
  const [fromLongitude, fromLatitude] = from;
  const [toLongitude, toLatitude] = to;
  const latitudeDelta = ((toLatitude - fromLatitude) * Math.PI) / 180;
  const longitudeDelta = ((toLongitude - fromLongitude) * Math.PI) / 180;
  const chord =
    Math.sin(latitudeDelta / 2) ** 2 +
    Math.cos((fromLatitude * Math.PI) / 180) *
      Math.cos((toLatitude * Math.PI) / 180) *
      Math.sin(longitudeDelta / 2) ** 2;

  return EARTH_RADIUS_METRES * 2 * Math.atan2(Math.sqrt(chord), Math.sqrt(1 - chord));
}

function elevationOf(position: Position): number | undefined {
  return position.length === 3 ? position[2] : undefined;
}

/**
 * Builds an evenly spaced profile, or null when the route carries no complete
 * elevation — a partial profile would imply flat ground where data is simply
 * absent.
 */
export function buildProfile(coordinates: Position[], sampleCount = 320): Profile | null {
  if (coordinates.length < 2 || coordinates.some((point) => elevationOf(point) === undefined)) {
    return null;
  }

  const distances: number[] = [0];
  for (let index = 1; index < coordinates.length; index++) {
    const previous = coordinates[index - 1];
    const current = coordinates[index];
    if (!previous || !current) {
      return null;
    }
    distances.push((distances[index - 1] ?? 0) + haversineMetres(previous, current));
  }

  const total = distances[distances.length - 1] ?? 0;
  if (total <= 0) {
    return null;
  }

  const sampled: Array<{ distanceMetres: number; elevationMetres: number }> = [];
  let cursor = 0;
  for (let step = 0; step < sampleCount; step++) {
    const target = (total * step) / (sampleCount - 1);
    while (cursor < distances.length - 2 && (distances[cursor + 1] ?? 0) < target) {
      cursor++;
    }
    const spanStart = distances[cursor] ?? 0;
    const spanEnd = distances[cursor + 1] ?? spanStart;
    const startElevation = elevationOf(coordinates[cursor] as Position) ?? 0;
    const endElevation = elevationOf(coordinates[cursor + 1] as Position) ?? startElevation;
    const span = spanEnd - spanStart;
    const ratio = span > 0 ? (target - spanStart) / span : 0;

    sampled.push({
      distanceMetres: target,
      elevationMetres: startElevation + ratio * (endElevation - startElevation),
    });
  }

  // Gradient is measured back over at least the window, never between adjacent
  // samples: on a short route the samples sit metres apart, where the figure
  // would describe altitude error rather than terrain.
  const samples: ProfileSample[] = sampled.map((sample, index) => {
    let behind = index;
    while (
      behind > 0 &&
      sample.distanceMetres - (sampled[behind]?.distanceMetres ?? 0) < GRADIENT_WINDOW_METRES
    ) {
      behind--;
    }
    const reference = sampled[behind];
    const run = reference ? sample.distanceMetres - reference.distanceMetres : 0;
    const rise = reference ? sample.elevationMetres - reference.elevationMetres : 0;
    const gradientPercent = run > 0 ? (rise / run) * 100 : 0;

    return { ...sample, gradientPercent, band: gradientBand(gradientPercent) };
  });

  const elevations = samples.map((sample) => sample.elevationMetres);

  return {
    samples,
    totalDistanceMetres: total,
    minElevationMetres: Math.min(...elevations),
    maxElevationMetres: Math.max(...elevations),
  };
}

/**
 * A round step close to `range / target`, so ticks land on readable numbers.
 *
 * The rungs are the usual 1 / 2 / 5 / 10, chosen by nearest rather than by
 * rounding up. Rounding up looks tidier in isolation but overshoots badly on
 * small ranges: a 7 km route asked for three ticks would jump to a step of 5 and
 * label only 0 and 5.
 */
export function niceStep(range: number, target: number): number {
  if (range <= 0 || target <= 0) {
    return 1;
  }
  const rough = range / target;
  const magnitude = 10 ** Math.floor(Math.log10(rough));
  const normalised = rough / magnitude;
  const step = normalised <= 1.5 ? 1 : normalised <= 3 ? 2 : normalised <= 7 ? 5 : 10;

  return step * magnitude;
}

/** Round tick values spanning [min, max] inclusive of the first step at or below min. */
export function ticksFor(min: number, max: number, target: number): number[] {
  const step = niceStep(max - min, target);
  const first = Math.ceil(min / step) * step;
  const ticks: number[] = [];
  for (let value = first; value <= max + step / 1000; value += step) {
    ticks.push(Number(value.toFixed(6)));
  }

  return ticks;
}
