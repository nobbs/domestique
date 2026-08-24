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

import type { BoundingBox, Position } from "../api/types";

const EARTH_RADIUS_METRES = 6_371_000;

/**
 * Gradient classes, gentlest first, in steps of three percent.
 *
 * Five, where there were three. Under three percent is ground a rider simply
 * rides over; above it the question is how hard, and one class for everything
 * from a steady six percent to a twelve percent wall refused to answer it. The
 * steps stay even rather than widening as they climb, and they stop at twelve:
 * past that a rider is out of the saddle whatever the number says, so the
 * resolution is spent on the range where the number still changes the ride.
 *
 * A class is named by the gradient it opens at rather than by the span it
 * covers — `6%` for six up to nine — because these labels are read as a row of
 * five chips, where five spelled-out ranges are five things to parse and five
 * opening numbers are a ramp. The span is still said in full wherever the class
 * is named in prose or to a screen reader, which is what `description` is for.
 *
 * Steepness gets a warm ramp — grey, then green, gold, orange and red — which
 * is a sequential scale's multi-hue exception: it means severity, and it carries
 * a key wherever it is drawn. Lightness falls with every step, so steeper reads
 * as "more" as well as "hotter".
 *
 * Colour is the only channel. Five steps cannot be told apart by hue alone under
 * red-green colour blindness, and the hatches that used to carry the difference
 * are gone: this service has one operator, who has said which channel they want
 * and that the ramp is theirs to read. Body text still clears 4.5:1 everywhere,
 * which is a separate promise and one the tests keep.
 *
 * The bands use absolute steepness, so a fast descent is marked as clearly as
 * the climb it mirrors.
 */
export const GRADIENT_BANDS = [
  { limit: 3, label: "flat", description: "under 3%" },
  { limit: 6, label: "3%", description: "3 to 6%" },
  { limit: 9, label: "6%", description: "6 to 9%" },
  { limit: 12, label: "9%", description: "9 to 12%" },
  { limit: Number.POSITIVE_INFINITY, label: "12%+", description: "12% and steeper" },
] as const;

/** The shortest span a gradient is measured over, matching the service. */
export const GRADIENT_WINDOW_METRES = 100;

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

/**
 * A point's elevation, if it carries one.
 *
 * Exported because a `Position` is a union of a pair and a triple, so reading
 * the third element is a narrowing question rather than an index — and every
 * reader that measures a rise has to answer it the same way.
 */
export function elevationOf(position: Position): number | undefined {
  return position.length === 3 ? position[2] : undefined;
}

/**
 * A run of the route that carries one steepness band, as segment indices.
 *
 * `endIndex` is where the *last segment* of the run starts, which is the same
 * convention the service's surface ranges use: a range covers the ground from
 * point `startIndex` to point `endIndex + 1`.
 */
export interface BandedRange {
  band: number;
  startIndex: number;
  endIndex: number;
}

/**
 * A run of one band shorter than this is absorbed into the run before it.
 *
 * The window a gradient is measured over is its own floor: a run shorter than
 * that was never sustained by the definition that classified it, and elevation
 * from a terrain model wobbles enough that a long steady four-percent drag
 * crosses the band edge repeatedly. Absorbed, the drag is reported as the one
 * thing it is.
 *
 * It is the same floor the service's `maxGradientPercent` uses, which is what
 * makes the summary line and the key agree by construction rather than by luck.
 */
const MIN_BAND_METRES = GRADIENT_WINDOW_METRES;

/**
 * The route as runs of one steepness band, in the order they are ridden.
 *
 * The one rule for a stage's steepness, and everything that speaks about bands
 * asks it: the key that lists them, the chart's columns, and the lines the map
 * paints. It bands the source coordinates rather than any resampling of them, so
 * which bands a stage has is a fact about the ground instead of about how many
 * samples a chart happened to ask for — a stage banded at whole-route zoom is
 * banded identically at two kilometres of it. Each caller runs it over the same
 * coordinates and gets the same runs back; nothing here is cached, and nothing
 * needs to be for the answers to agree.
 *
 * A segment is banded by the gradient measured back over
 * `GRADIENT_WINDOW_METRES`, never across the segment itself: source points can
 * be metres apart, and a rise of one metre over five is a fifth of a hillside,
 * not a twenty percent wall.
 *
 * Empty for geometry that is not fully surveyed, which is the same refusal
 * `buildProfile` makes — a map that banded the surveyed half and left the rest
 * flat would report missing data as level ground.
 */
export function gradientRanges(coordinates: Position[]): BandedRange[] {
  const lastIndex = coordinates.length - 1;
  if (lastIndex < 1 || coordinates.some((point) => elevationOf(point) === undefined)) {
    return [];
  }

  return bandedRanges(coordinates, cumulativeMetres(coordinates));
}

/** One band's share of a route, as a fraction from 0 to 1. */
export interface BandShare {
  band: number;
  share: number;
}

/**
 * The route's steepness as a strip, in the order it is ridden.
 *
 * One entry per run rather than one per band: a card's mix bar is read as a
 * profile at a glance — where the gentle kilometres are, and how the hard ones
 * are spread through them — which a total per band cannot say. Empty for
 * geometry the profile refuses, so a partly surveyed route shows no bar rather
 * than a flat one.
 */
export function gradientMix(coordinates: Position[]): BandShare[] {
  const ranges = gradientRanges(coordinates);
  if (ranges.length === 0) {
    return [];
  }
  const distances = cumulativeMetres(coordinates);
  const lastIndex = coordinates.length - 1;
  const total = distances[lastIndex] ?? 0;
  if (total <= 0) {
    return [];
  }

  return ranges.flatMap((range) => {
    // One point past the run, for the same reason the drawn lines take one: the
    // last point of a run is the first point of the run that takes over.
    const startMetres = distances[range.startIndex] ?? 0;
    const endMetres = distances[Math.min(range.endIndex + 1, lastIndex)] ?? startMetres;
    const share = (endMetres - startMetres) / total;

    return share > 0 ? [{ band: range.band, share }] : [];
  });
}

/**
 * The bands a stage has and how much of it each covers, gentlest first.
 *
 * What the key offers to pick out, and the figure it puts on every chip. It is
 * read off the stage's one classification, so the key offers no class the chart
 * has nothing to light and does not reshuffle underneath the reader's hand when
 * the chart is zoomed. A run too narrow to come out as a visible column at
 * whole-route zoom still counts: the stage has that ground whether or not this
 * many pixels can show it.
 *
 * Totalled per band rather than left as runs, which is the difference between
 * this and `gradientMix`. A key answers "how much of this ride is steep", and it
 * answers it once per class; the strip on a listing row answers "where", and has
 * to stay in the order the ground is ridden to do it.
 */
export function gradientShares(coordinates: Position[]): BandShare[] {
  const totals = new Map<number, number>();
  for (const entry of gradientMix(coordinates)) {
    totals.set(entry.band, (totals.get(entry.band) ?? 0) + entry.share);
  }

  return GRADIENT_BANDS.map((_, band) => band)
    .filter((band) => totals.has(band))
    .map((band) => ({ band, share: totals.get(band) ?? 0 }));
}

/** The classification proper, for a caller that has already measured the route. */
function bandedRanges(coordinates: Position[], distances: number[]): BandedRange[] {
  const lastIndex = coordinates.length - 1;
  const elevations = coordinates.map((point) => elevationOf(point) ?? 0);

  const runs: BandedRange[] = [];
  let behind = 0;
  for (let index = 1; index <= lastIndex; index++) {
    // The first point at least a full window back, or the start of the route.
    // The route's opening segments are measured over what there is, which is the
    // same shortfall the profile has at its own start.
    while (
      behind + 1 < index &&
      (distances[index] ?? 0) - (distances[behind + 1] ?? 0) >= GRADIENT_WINDOW_METRES
    ) {
      behind++;
    }
    const run = (distances[index] ?? 0) - (distances[behind] ?? 0);
    const rise = (elevations[index] ?? 0) - (elevations[behind] ?? 0);
    const band = gradientBand(run > 0 ? (rise / run) * 100 : 0);

    const current = runs[runs.length - 1];
    if (current && current.band === band) {
      current.endIndex = index - 1;

      continue;
    }
    runs.push({ band, startIndex: index - 1, endIndex: index - 1 });
  }

  return merged(runs, distances);
}

/**
 * Absorbs runs too short to be sustained into the run before them.
 *
 * Forwards in one pass, so a stipple of short runs collapses into the last band
 * that earned its place rather than into whichever of them happened to be
 * measured first. A short run at the very start has nothing to join and keeps
 * its own band; it is the one place where a wobble can still show, and it is
 * one run rather than a stipple.
 */
function merged(runs: BandedRange[], distances: number[]): BandedRange[] {
  const kept: BandedRange[] = [];
  for (const run of runs) {
    const length = (distances[run.endIndex + 1] ?? 0) - (distances[run.startIndex] ?? 0);
    const previous = kept[kept.length - 1];
    if (previous && length < MIN_BAND_METRES) {
      previous.endIndex = run.endIndex;

      continue;
    }
    if (previous && previous.band === run.band) {
      previous.endIndex = run.endIndex;

      continue;
    }
    kept.push({ ...run });
  }

  return kept;
}

/**
 * The band covering each of a list of ascending distances along the route.
 *
 * One cursor walks both lists, because the runs cover the route end to end in
 * order and the distances asked about are the samples of a profile, which do
 * too. A distance sitting exactly on a boundary belongs to the run that ends
 * there, which is also how the run drawn over it is cut.
 */
function bandsAt(ranges: readonly BandedRange[], distances: number[], targets: number[]): number[] {
  const ends = ranges.map((range) => distances[range.endIndex + 1] ?? 0);
  let cursor = 0;

  return targets.map((target) => {
    while (cursor < ends.length - 1 && (ends[cursor] ?? 0) < target) {
      cursor++;
    }

    return ranges[cursor]?.band ?? 0;
  });
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
  ranges: readonly BandedRange[],
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

  // Bands come from the stage's one classification rather than from these
  // samples, so the class under a stretch of chart does not change with the
  // spacing the chart was drawn at.
  const bands = bandsAt(
    ranges,
    distances,
    placed.map((sample) => sample.distanceMetres),
  );
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

    return { ...sample, gradientPercent, band: bands[index] ?? 0 };
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
): { distances: number[]; total: number; ranges: BandedRange[] } | null {
  if (sampleCount < 2) {
    return null;
  }
  if (coordinates.length < 2 || coordinates.some((point) => elevationOf(point) === undefined)) {
    return null;
  }
  const distances = cumulativeMetres(coordinates);
  const total = distances[distances.length - 1] ?? 0;

  return total > 0 ? { distances, total, ranges: bandedRanges(coordinates, distances) } : null;
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
    measured.ranges,
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

  return profileBetween(
    coordinates,
    measured.distances,
    measured.ranges,
    measured.total,
    start,
    end,
    sampleCount,
  );
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
 * The predicted moving time for one stretch of the route, read off the same
 * per-coordinate cumulative series the whole-stage figure comes from —
 * subtracting the selection's boundaries rather than a second computation.
 *
 * Undefined whenever there is nothing to subtract: no selection, no
 * predicted series for this geometry, or a selection too short to span two
 * distinct coordinates.
 */
export function elapsedSecondsForWindow(
  coordinates: Position[],
  cumulativeSeconds: number[] | undefined,
  window: DistanceWindow | null,
): number | undefined {
  if (!cumulativeSeconds || !window) {
    return undefined;
  }
  const range = coordinateRange(coordinates, window.startMetres, window.endMetres);
  if (!range) {
    return undefined;
  }
  const start = cumulativeSeconds[range.startIndex];
  const end = cumulativeSeconds[range.endIndex];
  if (start === undefined || end === undefined) {
    return undefined;
  }
  const elapsed = end - start;
  // A non-monotonic or duplicated series is a drift this client cannot
  // trust, not a stretch that took no time or ran backwards — reading it as
  // absence lets the panel fall back to the whole-stage figure instead of
  // rendering a zero or negative one.
  if (!Number.isFinite(elapsed) || elapsed <= 0) {
    return undefined;
  }

  return elapsed;
}

/**
 * The box that contains a stretch of the coordinates.
 *
 * Kept here, beside the range it is built from, rather than inside the map: the
 * map cannot be unit-tested, and framing the wrong ground is a mistake in
 * arithmetic rather than in rendering. It is deliberately only the geometry —
 * how much room to leave around it, and how close the camera may come, are the
 * map's business.
 *
 * A stretch of one point yields a box of no area. That is honest, and
 * `fitBounds` reads it as a place to centre on rather than as an error.
 */
export function rangeBounds(coordinates: Position[], range: CoordinateRange): BoundingBox | null {
  const first = coordinates[range.startIndex];
  if (!first) {
    return null;
  }
  let west = first[0];
  let south = first[1];
  let east = first[0];
  let north = first[1];

  for (let index = range.startIndex + 1; index <= range.endIndex; index++) {
    const point = coordinates[index];
    if (!point) {
      break;
    }
    west = Math.min(west, point[0]);
    south = Math.min(south, point[1]);
    east = Math.max(east, point[0]);
    north = Math.max(north, point[1]);
  }

  return [west, south, east, north];
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
