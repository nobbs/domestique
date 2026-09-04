/**
 * One forecast reading, where it falls on the route, and what the wind is doing
 * to the rider there.
 *
 * A reading is a moment, and the strip draws it as ground: a cell reaches from
 * halfway back to the reading before it to halfway on to the reading after, so
 * the tiles tile the route rather than sitting on it as points. The wind is
 * turned from a compass direction into a relation to the road, which needs the
 * geometry and so cannot be done by the endpoint that reports it.
 *
 * That turn is `relationAt`, and it is the only copy of it: the strip asks it
 * once per reading, and the route's own tint asks it every few hundred metres
 * along the way, because what changes between two readings is not the forecast
 * but the direction the road points in.
 *
 * The relation is not the *direction*, and the cell carries both. A relation
 * needs the road and inverts wherever the road turns; a direction is a fact
 * about the air and holds whatever the road does. `flowDegrees` is the second
 * of those, kept in the compass frame the map's own arrows use so that a tile
 * and the map cannot say the wind is going two different ways.
 */

import type { Position, WeatherPoint } from "../../api/types";
import type { ForecastSample } from "../../lib/forecastSamples";
import { windRelationStop } from "../../lib/measures";
import { cumulativeMetres } from "../../lib/profile";
import type { WindRelation } from "../../lib/wind";
import { BEARING_WINDOW_METRES, bearingAt, bearingIsMixed, windRelation } from "../../lib/wind";
import { flowBearingDegrees } from "../../lib/windField";

/** A relation, `"mixed"` where the road disagrees, null where there is no road. */
export type CellRelation = WindRelation | "mixed" | null;

/** How a wind sits against the road at one point along it. */
export interface RelationReading {
  relation: CellRelation;
  /** The wind's share along the direction of travel, -1 dead ahead to 1 dead behind. */
  component: number | null;
}

export interface Cell extends RelationReading {
  sample: ForecastSample;
  point: WeatherPoint;
  /**
   * Where the air is going, in degrees clockwise from north — a fact about the
   * reading alone, which is why it hangs off the cell rather than off
   * `relationAt` and needs no geometry to answer.
   */
  flowDegrees: number;
  /** The stretch this reading speaks for, halfway to each neighbour. */
  startMetres: number;
  endMetres: number;
}

/**
 * The wind's relation to the road at one distance along it, from a wind
 * reported the way a forecast reports it — the direction it blows *from*.
 *
 * `distances` is `cumulativeMetres(coordinates)`, taken from the caller for the
 * same reason `wind.ts` takes it: neither walks the geometry a second time.
 */
export function relationAt(
  coordinates: Position[],
  distances: number[],
  atMetres: number,
  windFromDegrees: number,
): RelationReading {
  const bearing = bearingAt(coordinates, distances, atMetres, BEARING_WINDOW_METRES);
  const mixed =
    bearing !== null && bearingIsMixed(coordinates, distances, atMetres, BEARING_WINDOW_METRES);
  const reading = bearing !== null ? windRelation(bearing, windFromDegrees) : null;

  return {
    relation: mixed ? "mixed" : (reading?.relation ?? null),
    component: reading?.componentKmhPerKmh ?? null,
  };
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

    return [
      {
        sample,
        point,
        startMetres: previous ? (previous.distanceMetres + sample.distanceMetres) / 2 : 0,
        endMetres: next ? (sample.distanceMetres + next.distanceMetres) / 2 : totalMetres,
        flowDegrees: flowBearingDegrees(point.windDirectionDegrees),
        ...relationAt(coordinates, distances, sample.distanceMetres, point.windDirectionDegrees),
      },
    ];
  });
}

/** How hard the wind is blowing, as a fraction of "hard enough to plan around". */
export function windWeight(kmh: number): number {
  return Math.min(kmh / 35, 1);
}

/**
 * How often the road's own direction is re-read while looking for what the wind
 * is doing to the rider to change.
 *
 * This scans the geometry, not the weather: the forecast holds for kilometres
 * at a time and the road does not. Finer than this buys nothing, because every
 * bearing is already averaged over `BEARING_WINDOW_METRES` of route either side
 * of the point it is taken at.
 */
export const RELATION_SCAN_METRES = 250;

/** The most reads one route gets, so the scan does not grow with the ride. */
const MAX_RELATION_SCANS = 2000;

/** One stretch of route the wind sits the same way against, end to end. */
export interface WindRun {
  fromMetres: number;
  toMetres: number;
  /** The stop on the head-to-tail ramp, and null for `"mixed"`. */
  stop: number | null;
  /** The wind's own speed here, from the reading whose stretch this falls in. */
  windSpeedKmh: number;
}

/**
 * The route cut into the stretches the wind sits the same way against.
 *
 * Cut at every cell boundary as well as at every change, because a reading
 * speaks for its own stretch and no further: a run that ran across a boundary
 * would carry a wind speed measured somewhere else. Neighbouring runs meet on
 * the distance the change was read at, so the runs tile the route.
 */
export function windRuns(cells: Cell[], coordinates: Position[], distances: number[]): WindRun[] {
  const totalMetres = distances[distances.length - 1] ?? 0;
  if (coordinates.length < 2 || totalMetres <= 0) {
    return [];
  }
  const interval = Math.max(RELATION_SCAN_METRES, totalMetres / MAX_RELATION_SCANS);
  const runs: WindRun[] = [];

  for (const cell of cells) {
    const span = cell.endMetres - cell.startMetres;
    const steps = Math.max(1, Math.ceil(span / interval));
    // The run this cell is still extending: a cell never extends the one before
    // it, which is what keeps every run's wind speed its own reading's.
    let openIndex = -1;
    for (let step = 0; step <= steps; step++) {
      const atMetres = cell.startMetres + (span * step) / steps;
      const reading = relationAt(coordinates, distances, atMetres, cell.point.windDirectionDegrees);
      // No bearing at all is "no road here", which is not the answer `"mixed"`
      // gives — so it opens nothing rather than being drawn as a neutral, and
      // the run open before this gap closes rather than silently stretching
      // across ground with no bearing to justify it.
      if (reading.relation === null) {
        openIndex = -1;
        continue;
      }
      const stop =
        reading.relation === "mixed"
          ? null
          : windRelationStop(reading.relation, reading.component ?? 0);
      const open = runs[openIndex];
      if (open && open.stop === stop) {
        open.toMetres = atMetres;

        continue;
      }
      // A change is read at this point, so the run before it ends here — not
      // wherever it was last extended to — and the new one starts at the same
      // distance, so the two share a boundary rather than a gap.
      if (open) {
        open.toMetres = atMetres;
      }
      runs.push({
        fromMetres: atMetres,
        toMetres: atMetres,
        stop,
        windSpeedKmh: cell.point.windSpeedKmh,
      });
      openIndex = runs.length - 1;
    }
  }

  return runs.filter((run) => run.toMetres > run.fromMetres);
}
