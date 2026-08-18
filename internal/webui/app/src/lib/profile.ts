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
 *
 * A profile describes a stretch of the route rather than always the whole of
 * it. Asked for a stretch, it samples that stretch at the full count instead of
 * handing back the few whole-route samples that fall inside it: a window
 * redrawn from those would magnify the sampling rather than the terrain.
 */

import type { Position } from "../api/types";

const EARTH_RADIUS_METRES = 6_371_000;

/**
 * Gradient classes, gentlest first.
 *
 * Three, not four. Steepness gets a warm heat ramp — gold through orange to
 * deep red — which is the multi-hue exception a sequential scale is allowed
 * when it means severity and carries a scale legend. Four steps could not be
 * made to separate: the top two were both red, differing only in lightness, and
 * sat around ΔE 9 where 15 is the floor for ordinary colour vision. Cutting a
 * class is the honest fix for a pair that will not separate; fudging the colours
 * would have left two bands nobody could tell apart.
 *
 * The green-amber-red scale cycling apps use was measured and rejected outright.
 * Making the amber yellow enough to read against the green collapses the pair to
 * ΔE 1.4–4.5 under protanopia: red-green colour blindness, which is common, makes
 * that scale unreadable exactly where it matters.
 *
 * The bands use absolute steepness, so a fast descent is marked as clearly as
 * the climb it mirrors.
 */
export const GRADIENT_BANDS = [
  { limit: 4, label: "< 4%" },
  { limit: 8, label: "4–8%" },
  { limit: Number.POSITIVE_INFINITY, label: "≥ 8%" },
] as const;

/** The shortest span a gradient is measured over, matching the service. */
const GRADIENT_WINDOW_METRES = 100;

/**
 * How far past either end of a profile a position may sit and still be found.
 *
 * A pointer at the very edge of the plot lands on the last sample give or take
 * a rounding error, and refusing it would blank the readout at exactly the
 * place a reader was aiming for.
 */
const POSITION_TOLERANCE_METRES = 0.5;

export function gradientBand(percent: number): number {
  const magnitude = Math.abs(percent);

  return GRADIENT_BANDS.findIndex((band) => magnitude < band.limit);
}

/**
 * A stretch of the route, in metres from its start.
 *
 * Metres rather than sample indices, because the same stretch has to mean the
 * same ground to a chart of the whole route, to a chart of two kilometres of
 * it, and to a map that holds no samples at all. An index means a different
 * place in each of the three.
 */
export interface DistanceWindow {
  startMetres: number;
  endMetres: number;
}

/** A stretch of a coordinate array, both ends inclusive. */
export interface CoordinateRange {
  startIndex: number;
  endIndex: number;
}

export interface ProfileSample {
  distanceMetres: number;
  elevationMetres: number;
  /**
   * Where this sample sits on the ground. The map and the chart address a
   * position by the distance along the route, so carrying the coordinate here
   * is what lets a hover on one show up on the other.
   */
  longitude: number;
  latitude: number;
  gradientPercent: number;
  band: number;
}

export interface Profile {
  samples: ProfileSample[];
  /**
   * The stretch these samples describe, in metres from the start of the route:
   * the whole route for an ordinary profile, and the window on show for a
   * zoomed one. The chart's distance axis runs between the two.
   */
  startMetres: number;
  endMetres: number;
  /** The whole route's length, which looking at part of it does not change. */
  totalDistanceMetres: number;
  /** Of the samples present, so a window's axis fits the ground it shows. */
  minElevationMetres: number;
  maxElevationMetres: number;
}

/**
 * Great-circle distance between two positions, on the same spherical model the
 * service uses. Exported because anything else measuring along a stage has to
 * agree with the profile's axis to the metre, and a second implementation would
 * eventually not.
 */
export function haversineMetres(from: Position, to: Position): number {
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

/**
 * Distance from the start of the route to each point.
 *
 * One implementation, shared by everything that places something along a stage.
 * The profile's axis and the surface classification's spans have to agree to the
 * metre or the strip would sit under the wrong climb, and two copies of this
 * loop would eventually not.
 */
export function cumulativeMetres(coordinates: Position[]): number[] {
  const distances = [0];
  for (let index = 1; index < coordinates.length; index++) {
    const previous = coordinates[index - 1];
    const current = coordinates[index];
    const travelled = distances[index - 1] ?? 0;
    distances.push(
      previous && current ? travelled + haversineMetres(previous, current) : travelled,
    );
  }

  return distances;
}

function elevationOf(position: Position): number | undefined {
  return position.length === 3 ? position[2] : undefined;
}

interface PlacedSample {
  distanceMetres: number;
  elevationMetres: number;
  longitude: number;
  latitude: number;
}

/**
 * Samples one stretch of a measured route, evenly by distance.
 *
 * Gradient is measured backwards over at least the window, never between
 * adjacent samples: on a short stretch the samples sit metres apart, where the
 * figure would describe altitude error rather than terrain. That look back
 * needs ground before the stretch begins, so the stretch is sampled with a
 * run-up which is thrown away before it is returned. Without it the opening
 * hundred metres of a window would be measured against nothing, and the
 * steepest pitch on a route would appear as flat ground the moment somebody
 * zoomed into it.
 *
 * A stretch starting at the route's own start gets no run-up, because there is
 * no ground before it — the same honest shortfall the whole route has always
 * had at its first sample.
 */
function profileBetween(
  coordinates: Position[],
  distances: number[],
  totalDistanceMetres: number,
  startMetres: number,
  endMetres: number,
  sampleCount: number,
): Profile | null {
  const span = endMetres - startMetres;
  if (span <= 0) {
    return null;
  }
  const step = span / (sampleCount - 1);
  const leadCount = Math.min(
    Math.ceil(GRADIENT_WINDOW_METRES / step),
    Math.floor(startMetres / step),
  );

  const placed: PlacedSample[] = [];
  let cursor = 0;
  for (let index = -leadCount; index < sampleCount; index++) {
    // The far end is pinned rather than accumulated, so the last sample lands
    // exactly on the edge of the stretch instead of a rounding error short of it.
    const target = index === sampleCount - 1 ? endMetres : startMetres + index * step;
    while (cursor < distances.length - 2 && (distances[cursor + 1] ?? 0) < target) {
      cursor++;
    }
    const spanStart = distances[cursor] ?? 0;
    const spanEnd = distances[cursor + 1] ?? spanStart;
    const from = coordinates[cursor] as Position;
    const to = (coordinates[cursor + 1] ?? from) as Position;
    const startElevation = elevationOf(from) ?? 0;
    const endElevation = elevationOf(to) ?? startElevation;
    const segment = spanEnd - spanStart;
    const ratio = segment > 0 ? (target - spanStart) / segment : 0;

    placed.push({
      distanceMetres: target,
      elevationMetres: startElevation + ratio * (endElevation - startElevation),
      longitude: from[0] + ratio * (to[0] - from[0]),
      latitude: from[1] + ratio * (to[1] - from[1]),
    });
  }

  const measured: ProfileSample[] = placed.map((sample, index) => {
    let behind = index;
    while (
      behind > 0 &&
      sample.distanceMetres - (placed[behind]?.distanceMetres ?? 0) < GRADIENT_WINDOW_METRES
    ) {
      behind--;
    }
    const reference = placed[behind];
    const run = reference ? sample.distanceMetres - reference.distanceMetres : 0;
    const rise = reference ? sample.elevationMetres - reference.elevationMetres : 0;
    const gradientPercent = run > 0 ? (rise / run) * 100 : 0;

    return { ...sample, gradientPercent, band: gradientBand(gradientPercent) };
  });

  const samples = measured.slice(leadCount);
  const elevations = samples.map((sample) => sample.elevationMetres);

  return {
    samples,
    startMetres,
    endMetres,
    totalDistanceMetres,
    minElevationMetres: Math.min(...elevations),
    maxElevationMetres: Math.max(...elevations),
  };
}

/** The measured route, or null when it carries no plottable elevation. */
function measure(
  coordinates: Position[],
  sampleCount: number,
): { distances: number[]; total: number } | null {
  if (sampleCount < 2) {
    return null;
  }
  if (coordinates.length < 2 || coordinates.some((point) => elevationOf(point) === undefined)) {
    return null;
  }
  const distances = cumulativeMetres(coordinates);
  const total = distances[distances.length - 1] ?? 0;

  return total > 0 ? { distances, total } : null;
}

/**
 * Builds an evenly spaced profile of the whole route, or null when it carries
 * no complete elevation — a partial profile would imply flat ground where data
 * is simply absent.
 *
 * Fewer than two samples is also null: the spacing divides by sampleCount - 1,
 * and one sample describes no span to plot.
 */
export function buildProfile(coordinates: Position[], sampleCount = 320): Profile | null {
  const measured = measure(coordinates, sampleCount);
  if (!measured) {
    return null;
  }

  return profileBetween(
    coordinates,
    measured.distances,
    measured.total,
    0,
    measured.total,
    sampleCount,
  );
}

/**
 * The same profile, restricted to one stretch of the route and sampled across
 * it at the full count.
 *
 * A named entry point rather than a third argument to `buildProfile`, because
 * the one call site that wants a window should say so.
 *
 * The window is clamped to the route rather than trusted: it arrives from a
 * pointer, and a drag that ran a pixel past the axis must not ask for ground
 * the route does not cover.
 */
export function buildWindowedProfile(
  coordinates: Position[],
  window: DistanceWindow,
  sampleCount = 320,
): Profile | null {
  const measured = measure(coordinates, sampleCount);
  if (!measured) {
    return null;
  }
  const start = Math.min(Math.max(window.startMetres, 0), measured.total);
  const end = Math.min(Math.max(window.endMetres, start), measured.total);

  return profileBetween(coordinates, measured.distances, measured.total, start, end, sampleCount);
}

/**
 * The sample at one distance along the stretch this profile describes, made by
 * interpolating between the two it falls between.
 *
 * Null outside that stretch. A zoomed chart describes a window, and a position
 * reported from somewhere else on the route is not one it can mark; saying so
 * plainly is what keeps a cursor from appearing at a place the chart is not
 * showing.
 *
 * Gradient and band are taken from the nearer of the two rather than blended.
 * A band is a class, and the average of two classes is not one of them.
 */
export function sampleAt(profile: Profile, metres: number): ProfileSample | null {
  const span = profile.endMetres - profile.startMetres;
  const lastIndex = profile.samples.length - 1;
  if (
    span <= 0 ||
    lastIndex < 1 ||
    metres < profile.startMetres - POSITION_TOLERANCE_METRES ||
    metres > profile.endMetres + POSITION_TOLERANCE_METRES
  ) {
    return null;
  }
  const position = Math.min(
    Math.max(((metres - profile.startMetres) / span) * lastIndex, 0),
    lastIndex,
  );
  const lower = Math.min(Math.floor(position), lastIndex - 1);
  const from = profile.samples[lower];
  const to = profile.samples[lower + 1];
  if (!from || !to) {
    return null;
  }
  const ratio = position - lower;
  const nearer = ratio < 0.5 ? from : to;

  return {
    distanceMetres: from.distanceMetres + ratio * (to.distanceMetres - from.distanceMetres),
    elevationMetres: from.elevationMetres + ratio * (to.elevationMetres - from.elevationMetres),
    longitude: from.longitude + ratio * (to.longitude - from.longitude),
    latitude: from.latitude + ratio * (to.latitude - from.latitude),
    gradientPercent: nearer.gradientPercent,
    band: nearer.band,
  };
}

/**
 * Where a stretch measured in metres begins and ends in a coordinate array.
 *
 * Rounded outwards — back to the last point at or before the start, on to the
 * first at or after the end — so what is drawn from the range covers every
 * metre the stretch asked for rather than stopping just inside it.
 */
export function coordinateRange(
  coordinates: Position[],
  startMetres: number,
  endMetres: number,
): CoordinateRange | null {
  if (coordinates.length < 2 || endMetres <= startMetres) {
    return null;
  }
  const distances = cumulativeMetres(coordinates);
  const lastIndex = coordinates.length - 1;

  let startIndex = 0;
  while (startIndex < lastIndex && (distances[startIndex + 1] ?? 0) <= startMetres) {
    startIndex++;
  }
  let endIndex = lastIndex;
  while (endIndex > startIndex && (distances[endIndex - 1] ?? 0) >= endMetres) {
    endIndex--;
  }

  return { startIndex, endIndex };
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

/** Round tick values spanning [min, max], starting at the first step at or above min. */
export function ticksFor(min: number, max: number, target: number): number[] {
  const step = niceStep(max - min, target);
  const first = Math.ceil(min / step) * step;
  const ticks: number[] = [];
  for (let value = first; value <= max + step / 1000; value += step) {
    ticks.push(Number(value.toFixed(6)));
  }

  return ticks;
}

/**
 * The sample nearest a point on the ground, or null when the point is nowhere
 * near the route.
 *
 * Longitude is scaled by the cosine of the latitude so a degree east counts for
 * what it is worth on the ground; without it, a point well north or south of the
 * route would match a sample that is nowhere near it.
 */
export function nearestSample(
  profile: Profile,
  longitude: number,
  latitude: number,
): number | null {
  const longitudeScale = Math.cos((latitude * Math.PI) / 180);
  let best: number | null = null;
  let bestDistance = Number.POSITIVE_INFINITY;

  profile.samples.forEach((sample, index) => {
    const east = (sample.longitude - longitude) * longitudeScale;
    const north = sample.latitude - latitude;
    const squared = east * east + north * north;
    if (squared < bestDistance) {
      bestDistance = squared;
      best = index;
    }
  });

  return best;
}
