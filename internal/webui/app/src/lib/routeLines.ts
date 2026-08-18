/**
 * The route cut into the pieces a map paints.
 *
 * Two encodings are drawn along the same line — the ground under the wheel and
 * how steeply it rises — and both are drawn as one set of lines per class. The
 * cutting rules they share live here so a boundary lands in the same place
 * whichever encoding asked for it: neighbouring pieces meet on one shared point
 * and share no segment, because a gap would break the drawn route and an
 * overlap would paint one metre of it twice.
 *
 * Steepness is measured here rather than read off the elevation profile. The
 * profile samples the route a few hundred times; a line drawn through those
 * samples would cut every bend, and an underlay that cuts bends shows as colour
 * spilling out of the corners of the route drawn over it.
 */

import type { Position } from "../api/types";
import type { CoordinateRange } from "./profile";
import { cumulativeMetres, elevationOf, GRADIENT_WINDOW_METRES, gradientBand } from "./profile";

/**
 * A run of the route that carries one value, as segment indices.
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

/** One band's stretches, split by whether they are drawn at full strength. */
export interface BandedSlices {
  band: number;
  inside: Position[][];
  outside: Position[][];
}

/**
 * The shortest run of one band worth drawing on its own.
 *
 * Elevation from a terrain model wobbles, so a long steady four-percent drag
 * crosses the band's edge repeatedly and would come out as a stipple of two
 * colours rather than as one stretch. A run shorter than this is given to the
 * band before it, which reports the drag as the one thing it is. The floor is
 * half the span a gradient is measured over: anything longer than that is a
 * change the measurement itself can see, rather than noise inside one window.
 */
const MIN_BAND_METRES = GRADIENT_WINDOW_METRES / 2;

/**
 * Cuts one stretch of the route into the parts drawn at full strength and the
 * parts left dim.
 *
 * What stays lit arrives as stretches rather than as one window because two
 * different questions dim the map: a chart zoomed into one stretch, and a class
 * picked out of the key, which is scattered along the whole ride. Null lights
 * everything, which is the map nobody has asked a question of.
 *
 * The lit stretches are expected in route order and disjoint apart from shared
 * end points, which is how every producer of them here reports ground.
 *
 * Neighbouring pieces share one point and no segment: a gap would break the
 * drawn route, and an overlap would paint one metre of it twice at two
 * different opacities. A piece of fewer than two points spans no ground and is
 * dropped, exactly as a range covering a single final point always was.
 */
export function splitAtRanges(
  coordinates: Position[],
  start: number,
  end: number,
  lit: readonly CoordinateRange[] | null,
): { inside: Position[][]; outside: Position[][] } {
  const piece = (from: number, to: number) => {
    const line = coordinates.slice(from, to + 1);

    return line.length < 2 ? [] : [line];
  };
  if (!lit) {
    return { inside: piece(start, end), outside: [] };
  }

  const inside: Position[][] = [];
  const outside: Position[][] = [];
  let cursor = start;
  for (const range of lit) {
    const from = Math.max(range.startIndex, start);
    const to = Math.min(range.endIndex, end);
    if (to <= from) {
      continue;
    }
    outside.push(...piece(cursor, from));
    inside.push(...piece(from, to));
    cursor = Math.max(cursor, to);
  }
  outside.push(...piece(cursor, end));

  return { inside, outside };
}

/**
 * The route itself split the same way, for the casing under the classes and for
 * a stage nobody has classified yet.
 */
export function routeLinesWithin(
  coordinates: Position[],
  lit: readonly CoordinateRange[] | null,
): { inside: Position[][]; outside: Position[][] } {
  const lastIndex = coordinates.length - 1;
  if (lastIndex < 1) {
    return { inside: [], outside: [] };
  }

  return splitAtRanges(coordinates, 0, lastIndex, lit);
}

/**
 * The route as runs of one steepness band, in the order they are ridden.
 *
 * A segment is banded by the gradient measured back over the same hundred
 * metres the profile uses, so the colour beside a stretch of route and the
 * colour under the same stretch of chart come from one rule. Measured backwards
 * rather than across the segment itself: source points can be metres apart, and
 * a rise of one metre over five is a fifth of a hillside, not a twenty percent
 * wall.
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
  const distances = cumulativeMetres(coordinates);
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
 * Absorbs runs too short to be anything but noise into the run before them.
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
 * The route's steepness as lines to paint, grouped by band and split by what is
 * drawn at full strength.
 *
 * Every band it finds, including the gentlest: which of them are worth ink is
 * the map's decision, not this one's.
 */
export function gradientSlices(
  coordinates: Position[],
  lit: readonly CoordinateRange[] | null,
): BandedSlices[] {
  const lastIndex = coordinates.length - 1;
  const slicesByBand = new Map<number, { inside: Position[][]; outside: Position[][] }>();
  for (const range of gradientRanges(coordinates)) {
    // One point past the range, because the last point of a run is the first
    // point of the run that takes over from it.
    const split = splitAtRanges(
      coordinates,
      range.startIndex,
      Math.min(range.endIndex + 1, lastIndex),
      lit,
    );
    const existing = slicesByBand.get(range.band);
    if (existing) {
      existing.inside.push(...split.inside);
      existing.outside.push(...split.outside);

      continue;
    }
    slicesByBand.set(range.band, split);
  }

  return [...slicesByBand.entries()]
    .sort(([left], [right]) => left - right)
    .map(([band, slices]) => ({ band, ...slices }));
}
