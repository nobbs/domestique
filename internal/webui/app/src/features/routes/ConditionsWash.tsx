/**
 * The forecast, washed along the ground it was asked about.
 *
 * One measure at a time — wind, temperature, rain or cloud — as a broad band of
 * colour hugging the route, under everything the route itself draws. It answers
 * what a strip of tiles cannot: *where* the rain starts, which side of the col
 * is the cold one. The strip stays the place to read one moment's numbers; this
 * is the place to see where a change lands on the map.
 *
 * Filled polygons, not a wide translucent line. A line kilometres wide folds
 * over itself on every bend tighter than its own half-width, and the two
 * translucent fragments blend into a dark streak that reads as weather nobody
 * forecast. `conditionsCorridor.ts` builds concentric rings that share no
 * ground at all instead, so nothing on the map is ever painted twice; the
 * stepped rings are what carry the fade the line's `line-blur` used to.
 *
 * The corridor is only as wide as the forecast's own grid cell allows
 * (`conditionsField.ts`), so a three-day-out forecast is drawn visibly broader
 * and vaguer than tomorrow morning's, and it fades from the core radius to the
 * edge rather than stopping on a line: a drawn boundary reads as a weather
 * front that is not there.
 *
 * Banded, never interpolated between bands. A colour half way between two bands
 * belongs to no band, and would quietly contradict the legend beside the map.
 *
 * The geometry is a `useMemo` over the route, the readings and the resolution,
 * and one `fill` layer paints all of it from properties on the features — so
 * there is no per-frame work here at all, and no layer per band either.
 *
 * Runs its own `weatherQuery`, the way `ForecastStrip` does: React Query keys on
 * the samples, so the two share one cache entry rather than fetching twice.
 */

import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { Layer, Source } from "react-map-gl/maplibre";
import { weatherQuery } from "../../api/queries";
import type { Position } from "../../api/types";
import { useCartography } from "../../components/map/CartographyContext";
import type { BandRun, CorridorRing } from "../../lib/conditionsCorridor";
import { corridorRings } from "../../lib/conditionsCorridor";
import type { ScalarSample } from "../../lib/conditionsField";
import { sampleScalarAt } from "../../lib/conditionsField";
import { forecastResolution } from "../../lib/forecastResolution";
import type { ForecastSample } from "../../lib/forecastSamples";
import { formatDistance } from "../../lib/format";
import type { Measure, MeasureKey } from "../../lib/measures";
import { MEASURES } from "../../lib/measures";
import { cumulativeMetres } from "../../lib/profile";
import type { UnitSystem } from "../../lib/units";

const SOURCE_ID = "route-conditions";

/** The wash's own layer, so callers ordering the stack have a name to use. */
export const WASH_LAYER_ID = "route-conditions-wash";

/**
 * How far apart the route is read while looking for a band change, in metres.
 *
 * The reading itself is smooth between forecast samples that are never closer
 * than five kilometres, so this is not sampling the weather — it is locating
 * the distance at which a band gives way to the next one. A hundred metres is a
 * few pixels at the zoom a whole route is framed at, and far finer than the
 * kilometres-wide grid cell the reading came from in the first place.
 */
export const BAND_SCAN_METRES = 100;

/**
 * The most reads any one route gets, which widens the interval on a very long
 * one rather than letting the scan grow with the ride.
 */
const MAX_BAND_SCANS = 4000;

/**
 * How strongly the wash paints where it paints at all.
 *
 * Ink on the ground rather than a coat over it: at full strength the corridor
 * hides the towns and roads the reader is placing the weather against, which
 * is the whole point of drawing it on a map instead of in the strip.
 */
export const WASH_OPACITY = 0.45;

/**
 * The stretches the route falls into, one per run of a single band.
 *
 * Read at a fine interval and cut only where the band changes, rather than one
 * run per forecast sample: the cut is the boundary itself, which is what keeps
 * the corridor banded rather than blended through a colour no reading is in.
 */
export function bandRuns(
  readings: ScalarSample[],
  totalMetres: number,
  measure: Measure,
): BandRun[] {
  const interval = Math.max(BAND_SCAN_METRES, totalMetres / MAX_BAND_SCANS);
  const starts: Array<{ metres: number; band: number }> = [];
  let previous: number | null = null;
  for (let metres = 0; metres <= totalMetres; metres += interval) {
    const value = sampleScalarAt(readings, metres);
    if (value === null) {
      continue;
    }
    const band = measure.band(value);
    if (band !== previous) {
      starts.push({ metres: Math.min(metres, totalMetres), band });
      previous = band;
    }
  }

  return starts.map((start, index) => ({
    band: start.band,
    fromMetres: start.metres,
    toMetres: starts[index + 1]?.metres ?? totalMetres,
  }));
}

/**
 * The rings as map features, each carrying the colour and the alpha it is to be
 * painted in.
 *
 * One layer reads both off the feature, rather than a layer per band: a paint
 * property is one value for a whole layer, and the corridor has a colour per
 * band and an alpha per ring of the fade.
 */
function washFeatures(rings: CorridorRing[], measure: Measure, dark: boolean) {
  return rings.map((ring, index) => ({
    type: "Feature" as const,
    id: index,
    geometry: { type: "MultiPolygon" as const, coordinates: ring.polygon },
    properties: {
      colour: measure.colour(ring.band, dark),
      opacity: WASH_OPACITY * ring.strength * measure.opacity(ring.band),
    },
  }));
}

export interface ConditionsWashProps {
  /** The route's stored geometry, which the wash is laid along. */
  coordinates: Position[];
  /**
   * The forecast requests for this ride. Empty until a start time is picked,
   * and empty for a stage nothing has predicted a moving time for — either way
   * there is no forecast to wash the route in.
   */
  samples: ForecastSample[];
  /** The measure the reader asked for, and null — the default — for none. */
  measure: MeasureKey | null;
  /** The layer the wash is drawn beneath, so it stays under the route itself. */
  beforeId?: string | undefined;
  /** The units the wash's written equivalent reports in. */
  unitSystem?: UnitSystem;
}

export function ConditionsWash({
  coordinates,
  samples,
  measure,
  beforeId,
  unitSystem = "metric",
}: ConditionsWashProps) {
  // The wash is ink on the map, so its ramp follows the loaded basemap rather
  // than the page's scheme — the same split `bandColour` and `bandVariable`
  // make between a mark on the ground and a swatch in a panel.
  const { dark: darkBasemap } = useCartography();
  const chosen = MEASURES.find((entry) => entry.key === measure) ?? null;
  // Only once a measure is asked for: the strip makes this same request under
  // the same key while it is on screen, so asking here shares its cache rather
  // than fetching twice — but with the wash off there is nothing to fetch for.
  const forecast = useQuery({
    ...weatherQuery(samples),
    enabled: samples.length > 0 && chosen !== null,
  });
  const points = forecast.data?.points;

  const distances = useMemo(() => cumulativeMetres(coordinates), [coordinates]);
  const totalMetres = distances[distances.length - 1] ?? 0;

  // One reading per sample, at the distance the sample sits at. `weatherQuery`
  // has already refused a response whose points do not match the samples 1:1.
  const readings = useMemo<ScalarSample[]>(
    () =>
      chosen && points
        ? samples.flatMap((sample, index) => {
            const point = points[index];

            return point
              ? [{ distanceMetres: sample.distanceMetres, value: chosen.reading(point) }]
              : [];
          })
        : [],
    [chosen, points, samples],
  );

  // How sharp this forecast can honestly be, from the lead time to the first
  // sample — the same figure the strip reports its resolution sentence from.
  const metresPerCell = useMemo(() => {
    const first = samples[0]?.arrivalAt;
    const leadHours = first ? Math.max(0, (first.getTime() - Date.now()) / 3_600_000) : 0;

    return forecastResolution(leadHours).metresPerCell;
  }, [samples]);

  const runs = useMemo(
    () =>
      chosen && readings.length > 0 && totalMetres > 0
        ? bandRuns(readings, totalMetres, chosen)
        : [],
    [chosen, readings, totalMetres],
  );

  // Once per route, never per frame. A band a measure paints nothing in — rain's
  // dry, cloud's clear — is dropped before any geometry is built for it, so
  // most of most rides costs nothing to draw.
  const wash = useMemo(() => {
    // A band a measure paints nothing in — rain's dry, cloud's clear — never
    // reaches the geometry at all, so most of most rides costs nothing to draw.
    const speaking = chosen ? runs.filter((run) => chosen.opacity(run.band) > 0) : [];

    return {
      type: "FeatureCollection" as const,
      features: chosen
        ? washFeatures(
            corridorRings(coordinates, distances, speaking, metresPerCell),
            chosen,
            darkBasemap,
          )
        : [],
    };
  }, [chosen, coordinates, darkBasemap, distances, metresPerCell, runs]);

  if (!chosen || runs.length === 0) {
    return null;
  }

  return (
    <>
      <Source id={SOURCE_ID} type="geojson" data={wash}>
        <Layer
          id={WASH_LAYER_ID}
          type="fill"
          // Spread only when there is one. Under `exactOptionalPropertyTypes`
          // an optional prop and one that may be undefined are different types.
          {...(beforeId === undefined ? {} : { beforeId })}
          paint={{
            "fill-color": ["get", "colour"],
            // Per feature, not per layer: one layer paints every band and every
            // ring of the fade, and the ring itself carries how strong it is.
            "fill-opacity": ["get", "opacity"],
            // The outline MapLibre draws to antialias an edge would paint the
            // seam between two rings twice, which is the one thing the whole
            // corridor is built to avoid.
            "fill-antialias": false,
          }}
        />
      </Source>
      {/*
       * The wash in words. It is painted into a WebGL surface that carries no
       * text, so for a reader who is not looking at the canvas this is not a
       * caption — it is the whole of what the wash says, and it is why colour is
       * never the only channel this encoding has.
       */}
      <table className="visually-hidden">
        <caption>{chosen.label} along the route</caption>
        <thead>
          <tr>
            <th scope="col">Distance</th>
            <th scope="col">{chosen.label}</th>
          </tr>
        </thead>
        <tbody>
          {readings.map((reading) => (
            <tr key={reading.distanceMetres}>
              <th scope="row">{formatDistance(reading.distanceMetres, unitSystem)}</th>
              <td>{chosen.words(reading.value, unitSystem)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
}
