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
 * The series averaged over the ground each profile sample stands for: every
 * reading from halfway back to halfway on, which is the stretch the sample is
 * drawn across.
 *
 * A ride records once a second and the profile holds a few hundred samples, so
 * a reading taken at one of them describes one second out of several hundred
 * metres — a single coasted second draws a cadence spike to zero across ground
 * the rider pedalled. The mean describes the stretch; the pick described the
 * instant it landed on.
 *
 * A gap the sensor left is still a gap: nulls are left out of the mean, and a
 * stretch that recorded nothing at all stays null rather than borrowing from
 * its neighbours. A stretch holding no reading at all — the samples sit closer
 * together than the recording does — falls back to the nearest reading, which
 * is what it would have been given before.
 */
export function alignSeries(
  values: (number | null)[],
  coordinates: Position[],
  profile: Profile,
): (number | null)[] {
  const distances = cumulativeMetres(coordinates);
  const samples = profile.samples;
  const span = (samples.at(-1)?.distanceMetres ?? 0) - (samples[0]?.distanceMetres ?? 0);
  const halfStep = samples.length > 1 ? span / (samples.length - 1) / 2 : 0;

  let cursor = 0;

  return samples.map((sample, index) => {
    const previous = samples[index - 1]?.distanceMetres;
    const next = samples[index + 1]?.distanceMetres;
    // The end buckets are half-steps like the rest, never open: the profile
    // starts where the altitudes do, and ground before it belongs to no sample.
    const to =
      next === undefined ? sample.distanceMetres + halfStep : (sample.distanceMetres + next) / 2;
    const from =
      previous === undefined
        ? sample.distanceMetres - halfStep
        : (previous + sample.distanceMetres) / 2;

    while (cursor < distances.length && (distances[cursor] as number) < from) {
      cursor++;
    }
    let total = 0;
    let count = 0;
    for (let at = cursor; at < distances.length && (distances[at] as number) < to; at++) {
      const value = values[at];
      if (value !== null && value !== undefined) {
        total += value;
        count++;
      }
    }
    if (count > 0) {
      return total / count;
    }

    // Only a stretch that recorded nothing, or nothing but gaps, gets here.
    return values[nearestIndex(distances, sample.distanceMetres)] ?? null;
  });
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
