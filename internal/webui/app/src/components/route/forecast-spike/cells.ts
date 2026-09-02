/**
 * What every layout below is drawing: one reading, where it falls on the
 * route, and what the wind is doing to the rider there.
 *
 * A copy of the arithmetic inside `ForecastStrip`, deliberately. The spike is
 * meant to be deleted with one `rm -rf`, and four layouts reaching into the
 * component they are competing with would leave it holding a shape chosen for
 * the losers. When one wins, its promotion is where the two become one.
 */

import type { Position, WeatherPoint } from "../../../api/types";
import type { ForecastSample } from "../../../lib/forecastSamples";
import { cumulativeMetres } from "../../../lib/profile";
import type { WindRelation } from "../../../lib/wind";
import { BEARING_WINDOW_METRES, bearingAt, bearingIsMixed, windRelation } from "../../../lib/wind";

/** As `ForecastStrip`: a relation, `"mixed"` where the road disagrees, null where there is no road. */
export type CellRelation = WindRelation | "mixed" | null;

export interface Cell {
  sample: ForecastSample;
  point: WeatherPoint;
  /** The stretch this reading speaks for, halfway to each neighbour. */
  startMetres: number;
  endMetres: number;
  relation: CellRelation;
  /** The wind's share along the direction of travel, -1 dead ahead to 1 dead behind. */
  component: number | null;
  /** Which way the wind pushes, in degrees clockwise from the way the rider faces. */
  pushDegrees: number | null;
}

export function buildCells(
  samples: ForecastSample[],
  points: WeatherPoint[],
  coordinates: Position[],
): Cell[] {
  const distances = cumulativeMetres(coordinates);
  const totalMetres = distances[distances.length - 1] ?? 0;

  return samples.flatMap((sample, index) => {
    const point = points[index];
    if (!point) {
      return [];
    }
    const previous = samples[index - 1];
    const next = samples[index + 1];
    const bearing = bearingAt(coordinates, distances, sample.distanceMetres, BEARING_WINDOW_METRES);
    const mixed =
      bearing !== null &&
      bearingIsMixed(coordinates, distances, sample.distanceMetres, BEARING_WINDOW_METRES);
    const reading = bearing !== null ? windRelation(bearing, point.windDirectionDegrees) : null;

    return [
      {
        sample,
        point,
        startMetres: previous ? (previous.distanceMetres + sample.distanceMetres) / 2 : 0,
        endMetres: next ? (sample.distanceMetres + next.distanceMetres) / 2 : totalMetres,
        relation: (mixed ? "mixed" : (reading?.relation ?? null)) as CellRelation,
        component: reading?.componentKmhPerKmh ?? null,
        /*
         * A forecast names the direction wind comes *from*; a rider cares
         * which way it shoves them, which is the opposite. Measured against
         * the way they are pointing rather than against north, so an arrow
         * drawn at this angle reads as "behind you" or "in your face" without
         * the reader holding a compass in their head.
         */
        pushDegrees:
          bearing === null ? null : (point.windDirectionDegrees + 180 - bearing + 360) % 360,
      },
    ];
  });
}

/** How hard the wind is blowing, as a fraction of "hard enough to plan around". */
export function windWeight(kmh: number): number {
  return Math.min(kmh / 35, 1);
}
