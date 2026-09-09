/**
 * A ride's recorded sensor series, laid onto the elevation profile's own axis.
 *
 * The service serves a series indexed by coordinate; the profile is sampled
 * evenly by distance, and drops the coordinates that carried no altitude. So
 * the two are never index-for-index, and a series plotted by index would sit
 * under the wrong ground. Both agree on distance from the start, which is the
 * axis this maps onto.
 */

import type { Position } from "../api/types";
import { cumulativeMetres, type Profile } from "./profile";

/** One series ready to draw: its values on the profile's own samples. */
export interface AlignedSeries {
  key: string;
  label: string;
  unit: string;
  /** How many decimals a reading is shown with. Absent reads as a whole number. */
  decimals?: number;
  colour: string;
  values: (number | null)[];
}

/**
 * The series' value at each profile sample, taken from the nearest coordinate
 * by distance rather than interpolated: a gap the sensor left stays a gap,
 * and averaging across one would invent a reading over it.
 */
export function alignSeries(
  values: (number | null)[],
  coordinates: Position[],
  profile: Profile,
): (number | null)[] {
  const distances = cumulativeMetres(coordinates);

  return profile.samples.map(
    (sample) => values[nearestIndex(distances, sample.distanceMetres)] ?? null,
  );
}

/** The index of the distance closest to metres, by binary search. */
function nearestIndex(distances: number[], metres: number): number {
  let low = 0;
  let high = distances.length - 1;
  while (low < high) {
    const middle = (low + high) >> 1;
    if ((distances[middle] ?? 0) < metres) {
      low = middle + 1;
    } else {
      high = middle;
    }
  }
  const before = low - 1;
  if (before >= 0 && metres - (distances[before] ?? 0) < (distances[low] ?? 0) - metres) {
    return before;
  }

  return low;
}
