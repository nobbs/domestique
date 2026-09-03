/**
 * The forecast, washed along the ground it was asked about.
 *
 * One measure at a time — wind, temperature, rain or cloud — as a broad band of
 * colour hugging the route, under everything the route itself draws. It answers
 * what a strip of tiles cannot: *where* the rain starts, which side of the col
 * is the cold one. The strip stays the place to read one moment's numbers; this
 * is the place to see where a change lands on the map.
 *
 * A `line-gradient` over one LineString, not a canvas and not a custom layer.
 * The wash is a function of distance along the route, which is precisely what
 * `line-progress` is, and the corridor's width and its soft edge are the line's
 * own `line-width` and `line-blur` — so there is no per-frame work here at all.
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
 * Runs its own `weatherQuery`, the way `ForecastStrip` does: React Query keys on
 * the samples, so the two share one cache entry rather than fetching twice.
 */

import { useQuery } from "@tanstack/react-query";
import type { DataDrivenPropertyValueSpecification, ExpressionSpecification } from "maplibre-gl";
import { useMemo } from "react";
import { Layer, Source } from "react-map-gl/maplibre";
import { weatherQuery } from "../../api/queries";
import type { Position } from "../../api/types";
import { useCartography } from "../../components/map/CartographyContext";
import type { ScalarSample } from "../../lib/conditionsField";
import { corridorRadii, sampleScalarAt } from "../../lib/conditionsField";
import { forecastResolution } from "../../lib/forecastResolution";
import type { ForecastSample } from "../../lib/forecastSamples";
import { formatDistance } from "../../lib/format";
import type { Measure, MeasureKey } from "../../lib/measures";
import { MEASURES } from "../../lib/measures";
import { cumulativeMetres } from "../../lib/profile";
import { metresPerPixel } from "../../lib/routeCues";
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

/** MapLibre's own highest zoom, and so the far end of every ground-width ramp. */
const MAX_ZOOM = 22;

/**
 * A width fixed on the ground, as a paint expression in screen pixels.
 *
 * Metres per pixel halves with every zoom level, so a constant ground width is
 * exactly an exponential-base-2 ramp and two stops pin it at every zoom — no
 * recomputation as the camera moves. The latitude is the route's own, which
 * only enters through Mercator's cosine and cannot vary meaningfully over the
 * few tenths of a degree one ride spans.
 */
function groundWidth(
  metres: number,
  latitude: number,
): DataDrivenPropertyValueSpecification<number> {
  const atZoomZero = metres / metresPerPixel(0, latitude);

  return [
    "interpolate",
    ["exponential", 2],
    ["zoom"],
    0,
    atZoomZero,
    MAX_ZOOM,
    atZoomZero * 2 ** MAX_ZOOM,
  ];
}

/**
 * A band's colour with its opacity baked into the alpha channel.
 *
 * `line-opacity` is one number for the whole layer, so a measure whose lowest
 * band paints nothing — rain's dry, cloud's clear — can only say so here. Baked
 * as alpha, that band is genuinely absent rather than painted pale.
 */
function washColour(measure: Measure, band: number, dark: boolean): string {
  const hex = measure.colour(band, dark);
  const value = Number.parseInt(hex.slice(1), 16);

  return `rgba(${(value >> 16) & 255}, ${(value >> 8) & 255}, ${value & 255}, ${measure.opacity(band)})`;
}

/** One band, and the fraction of the route's length it takes over at. */
interface BandStop {
  progress: number;
  band: number;
}

/**
 * Where each band starts, as fractions of the route's length.
 *
 * Read at a fine interval and emitted only where the band changes, rather than
 * one stop per forecast sample: the stops are the boundaries themselves, which
 * is what lets the gradient be a `step` rather than an interpolation through
 * colours that are in no band at all.
 */
function bandStops(readings: ScalarSample[], totalMetres: number, measure: Measure): BandStop[] {
  const interval = Math.max(BAND_SCAN_METRES, totalMetres / MAX_BAND_SCANS);
  const stops: BandStop[] = [];
  let previous: number | null = null;
  for (let metres = 0; metres <= totalMetres; metres += interval) {
    const value = sampleScalarAt(readings, metres);
    if (value === null) {
      continue;
    }
    const band = measure.band(value);
    if (band !== previous) {
      stops.push({ progress: Math.min(metres / totalMetres, 1), band });
      previous = band;
    }
  }

  return stops;
}

/**
 * The stops as a `step` over `line-progress`.
 *
 * A route in one band still needs a stop pair — MapLibre's `step` takes no
 * fewer — so it repeats its own colour at the finish. Cast because a variadic
 * expression does not narrow to the tuple union the spec is typed as, the same
 * reason `RouteOverlay`'s `dimmedOutside` is written as a function.
 */
function washGradient(stops: BandStop[], measure: Measure, dark: boolean): ExpressionSpecification {
  const [first, ...rest] = stops;
  if (!first) {
    throw new Error("a wash gradient needs at least one band");
  }
  const tail =
    rest.length > 0
      ? rest.flatMap((stop) => [stop.progress, washColour(measure, stop.band, dark)])
      : [1, washColour(measure, first.band, dark)];

  return [
    "step",
    ["line-progress"],
    washColour(measure, first.band, dark),
    ...tail,
  ] as unknown as ExpressionSpecification;
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

  const stops = useMemo(
    () =>
      chosen && readings.length > 0 && totalMetres > 0
        ? bandStops(readings, totalMetres, chosen)
        : [],
    [chosen, readings, totalMetres],
  );

  // One LineString, because `line-progress` runs from 0 to 1 over one line and
  // a multi-part geometry would restart the gradient on each part.
  const line = useMemo(
    () => ({
      type: "Feature" as const,
      geometry: { type: "LineString" as const, coordinates },
      properties: {},
    }),
    [coordinates],
  );

  if (!chosen || stops.length === 0) {
    return null;
  }

  const { coreMetres, edgeMetres } = corridorRadii(metresPerCell);
  const latitude = coordinates[Math.floor(coordinates.length / 2)]?.[1] ?? 0;

  return (
    <>
      <Source id={SOURCE_ID} type="geojson" lineMetrics data={line}>
        <Layer
          id={WASH_LAYER_ID}
          type="line"
          // Spread only when there is one. Under `exactOptionalPropertyTypes`
          // an optional prop and one that may be undefined are different types.
          {...(beforeId === undefined ? {} : { beforeId })}
          layout={{ "line-cap": "butt", "line-join": "round" }}
          paint={{
            "line-gradient": washGradient(stops, chosen, darkBasemap),
            // The corridor's full width: twice the radius at which a reading
            // has faded to nothing.
            "line-width": groundWidth(edgeMetres * 2, latitude),
            // Full strength out to the core radius, gone at the edge, which is
            // what leaves the corridor with no boundary of its own to read.
            "line-blur": groundWidth(edgeMetres - coreMetres, latitude),
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
