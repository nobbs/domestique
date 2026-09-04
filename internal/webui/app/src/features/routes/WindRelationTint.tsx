/**
 * The route drawn in what the wind is doing to the rider, head to tail.
 *
 * The corridor beside it (`ConditionsWash`) answers how hard the wind blows;
 * this answers the half of the question a speed cannot: whether it is pushing
 * the rider along or into their face. That depends on which way the road points,
 * so it is drawn on the road rather than around it, in a diverging ramp from
 * headwind to tailwind.
 *
 * In the edging slot under the casing — the one steepness usually has, which is
 * why `RouteOverlay` gives that up while this is drawn. One encoding to a line:
 * the class the wheel is on stays on the line above the casing, and the reader
 * is never left working out which of two ramps a colour belongs to.
 *
 * `"mixed"` is drawn in a neutral rather than in the middle of the ramp. A
 * switchback is honestly heading two ways at once and `wind.ts` refuses to
 * average it, so the mark for it has to read as no confident answer rather than
 * as a confident crosswind.
 *
 * Runs its own `weatherQuery`, the way `ConditionsWash` and `ForecastStrip` do:
 * React Query keys on the samples, so all three share one cache entry.
 */

import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { Layer, Source } from "react-map-gl/maplibre";
import { weatherQuery } from "../../api/queries";
import type { Position } from "../../api/types";
import { useCartography } from "../../components/map/CartographyContext";
import type { ForecastSample } from "../../lib/forecastSamples";
import { formatDistance } from "../../lib/format";
import { windRelationColour, windRelationWords } from "../../lib/measures";
import type { CoordinateRange } from "../../lib/profile";
import { cumulativeMetres } from "../../lib/profile";
import { distanceSlices } from "../../lib/routeLines";
import type { UnitSystem } from "../../lib/units";
import type { WindRun } from "./forecastCells";
import { buildCells, windRuns } from "./forecastCells";
import { dimmedOutside, EDGING_WIDTH, taggedCollection } from "./routeFeatures";

const SOURCE_ID = "route-wind-relation";

/** The tint's own layer, so a caller ordering the stack has a name to use. */
export const WIND_TINT_LAYER_ID = "route-wind-relation-line";

/**
 * The stretches the wind sits the same way against, or none at all.
 *
 * A hook rather than work inside the component below, because whether anything
 * is tinted is not this component's business alone: the overlay puts its
 * steepness edging away for as long as the tint has the slot, and it can only
 * do that if it knows the answer before it draws.
 */
export function useWindRuns(
  samples: ForecastSample[],
  coordinates: Position[],
  enabled: boolean,
): WindRun[] {
  const forecast = useQuery({
    ...weatherQuery(samples),
    enabled: enabled && samples.length > 0,
  });
  const points = forecast.data?.points;
  const distances = useMemo(() => cumulativeMetres(coordinates), [coordinates]);

  return useMemo(
    () =>
      enabled && points
        ? windRuns(buildCells(samples, points, coordinates), coordinates, distances)
        : [],
    [coordinates, distances, enabled, points, samples],
  );
}

export interface WindRelationTintProps {
  /** The stretches to draw, from `useWindRuns`. Empty draws nothing at all. */
  runs: WindRun[];
  coordinates: Position[];
  /** The ground left lit, exactly as every other layer on the route reads it. */
  lit?: readonly CoordinateRange[] | null;
  /** The layer the tint is drawn beneath, so it stays under the casing. */
  beforeId?: string | undefined;
  /** The units the tint's written equivalent reports in. */
  unitSystem?: UnitSystem;
}

export function WindRelationTint({
  runs,
  coordinates,
  lit = null,
  beforeId,
  unitSystem = "metric",
}: WindRelationTintProps) {
  // The tint is ink on the map, so its ramp follows the loaded basemap rather
  // than the page's scheme — the same split every other mark on the route makes.
  const { dark: darkBasemap } = useCartography();
  const distances = useMemo(() => cumulativeMetres(coordinates), [coordinates]);

  // One layer for all of them, each stretch carrying its own colour, rather
  // than a layer per stop: a stop has no width or dash of its own to own, and
  // one layer is one place in the stack whatever the wind does.
  const tint = useMemo(() => {
    const slices = distanceSlices(coordinates, distances, runs, lit);

    return {
      type: "FeatureCollection" as const,
      features: runs.flatMap(
        (run, index) =>
          taggedCollection(slices[index] ?? { inside: [], outside: [] }, {
            colour: windRelationColour(run.stop, darkBasemap),
          }).features,
      ),
    };
  }, [coordinates, darkBasemap, distances, lit, runs]);

  if (runs.length === 0) {
    return null;
  }

  return (
    <>
      <Source id={SOURCE_ID} type="geojson" data={tint}>
        <Layer
          id={WIND_TINT_LAYER_ID}
          type="line"
          // Spread only when there is one. Under `exactOptionalPropertyTypes`
          // an optional prop and one that may be undefined are different types.
          {...(beforeId === undefined ? {} : { beforeId })}
          // Butt caps, so a stretch ends on the distance it hands over at
          // rather than half a line width into the one that follows it.
          layout={{ "line-cap": "butt", "line-join": "round" }}
          paint={{
            "line-color": ["get", "colour"],
            "line-width": EDGING_WIDTH,
            "line-opacity": dimmedOutside(1, lit !== null),
          }}
        />
      </Source>
      {/*
       * The tint in words. It is painted into a WebGL surface that carries no
       * text, so for a reader who is not looking at the canvas this is not a
       * caption — it is the whole of what the tint says, and it is why colour is
       * never the only channel this encoding has.
       *
       * Its own table rather than a column beside the corridor's: these are the
       * stretches the road holds one direction for, which are not the moments
       * the forecast was asked about.
       */}
      <table className="visually-hidden">
        <caption>Wind against the way you are riding</caption>
        <thead>
          <tr>
            <th scope="col">Distance</th>
            <th scope="col">Wind</th>
          </tr>
        </thead>
        <tbody>
          {runs.map((run, index) => (
            <tr key={`${run.fromMetres}-${index}`}>
              <th scope="row">
                {formatDistance(run.fromMetres, unitSystem)}
                {"–"}
                {formatDistance(run.toMetres, unitSystem)}
              </th>
              <td>{windRelationWords(run.stop, run.windSpeedKmh, unitSystem)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
}
