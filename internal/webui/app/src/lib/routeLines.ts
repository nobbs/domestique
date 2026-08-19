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
 * Steepness itself is classified once, in `profile`, from the source
 * coordinates — so the colour under a stretch of route, the column under the
 * same stretch of chart, and the chip in the key all come from one pass. What is
 * left here is the cutting: the runs it reports are point indices, and a line
 * drawn through the profile's few hundred samples instead would cut every bend,
 * which shows as colour spilling out of the corners of the route drawn over it.
 */

import type { Position } from "../api/types";
import type { CoordinateRange } from "./profile";
import { gradientRanges } from "./profile";

/** One band's stretches, split by whether they are drawn at full strength. */
export interface BandedSlices {
  band: number;
  inside: Position[][];
  outside: Position[][];
}

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
