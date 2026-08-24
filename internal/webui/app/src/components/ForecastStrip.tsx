/**
 * A stage's forecast, as a strip of cells under the elevation chart: one per
 * forecast sample, in the order the ride reaches them.
 *
 * Colour carries precipitation — the one figure worth reading at a glance —
 * but nothing here is colour-only. `ElevationProfile`'s bands can afford that
 * because this service has one operator who has said colour is the channel
 * they want; a forecast a rider might actually plan a ride's start time
 * around does not get that exemption. So every value a cell encodes is also
 * reachable as text: a letter glyph for the wind relation, an SVG `<title>`
 * a pointer can hover for the exact figures, and a visually hidden `<table>`
 * beside the strip that a screen reader reads instead of the graphic — the
 * `role="img"` on the `<svg>` carries one summary label and cannot carry
 * forty-eight readings on its own.
 *
 * The wind reading can also be honestly unsettled. A switchback is heading
 * two directions within a few hundred metres, and `wind.ts` reports that as
 * `"mixed"` rather than averaging it into a confident crosswind; this draws
 * that as its own glyph rather than picking one of the two directions to
 * believe.
 *
 * Runs its own query — `MapCredits` is this component's own precedent for
 * that — so a caller only has to hand over what the forecast is asked about,
 * not thread a query result through props.
 */

import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { ApiError } from "../api/client";
import { weatherQuery } from "../api/queries";
import type { Position, WeatherPoint } from "../api/types";
import type { ForecastSample } from "../lib/forecastSamples";
import { formatPrecipitation, formatTemperature, formatWindSpeed } from "../lib/format";
import { PADDING, plotAxis } from "../lib/plotAxis";
import { cumulativeMetres } from "../lib/profile";
import type { UnitSystem } from "../lib/units";
import { useElementWidth } from "../lib/useElementWidth";
import type { WindRelation } from "../lib/wind";
import { BEARING_WINDOW_METRES, bearingAt, bearingIsMixed, windRelation } from "../lib/wind";

/**
 * What a cell's own glyph and table row say about the wind: one of
 * `wind.ts`'s own relations, `"mixed"` for a window that disagreed with
 * itself, or null when there was no direction of travel to measure at all.
 */
type CellRelation = WindRelation | "mixed" | null;

/** How tall the strip's row of cells is drawn. Short: it is a strip, not a chart. */
const STRIP_HEIGHT = 22;

/** Below this many pixels a cell's own wind glyph is dropped rather than crowded. */
const MIN_GLYPH_CELL_WIDTH = 14;

/**
 * How far past 0% probability the precipitation fill climbs, and how far it
 * is allowed to go. A cell with no chance of rain draws no fill at all —
 * "colour carries precipitation" means an absence of colour is an honest
 * answer too.
 */
const MAX_PRECIPITATION_OPACITY = 0.55;

/**
 * Open-Meteo's WMO weather codes, the subset this service's own forecast
 * ever answers with (`internal/httpapi/routes_weather.go` passes the
 * provider's own code straight through). A code not in this table is
 * reported by number rather than guessed at.
 */
const WEATHER_CODE_LABELS: Record<number, string> = {
  0: "clear sky",
  1: "mainly clear",
  2: "partly cloudy",
  3: "overcast",
  45: "fog",
  48: "depositing rime fog",
  51: "light drizzle",
  53: "moderate drizzle",
  55: "dense drizzle",
  56: "light freezing drizzle",
  57: "dense freezing drizzle",
  61: "slight rain",
  63: "moderate rain",
  65: "heavy rain",
  66: "light freezing rain",
  67: "heavy freezing rain",
  71: "slight snow",
  73: "moderate snow",
  75: "heavy snow",
  77: "snow grains",
  80: "slight rain showers",
  81: "moderate rain showers",
  82: "violent rain showers",
  85: "slight snow showers",
  86: "heavy snow showers",
  95: "thunderstorm",
  96: "thunderstorm with slight hail",
  99: "thunderstorm with heavy hail",
};

const COMPASS_POINTS = ["N", "NE", "E", "SE", "S", "SW", "W", "NW"];

function weatherCodeLabel(code: number): string {
  return WEATHER_CODE_LABELS[code] ?? `weather code ${code}`;
}

function compassLabel(degrees: number): string {
  const index = Math.round((((degrees % 360) + 360) % 360) / 45) % 8;

  return COMPASS_POINTS[index] ?? "N";
}

/**
 * How much a forecast this far ahead can be trusted to be precise, in one
 * line rather than per cell: the response carries no model name, so this
 * infers it from lead time the way an operator reading Open-Meteo's own
 * documentation would. Within about two days it is ICON-D2 at roughly 2 km;
 * beyond that, ICON-EU or the global model at 7–11 km; past 78 hours, coarser
 * again. A forecast eleven days out looks exactly as confident on this strip
 * as one for tomorrow morning, and it is not.
 */
function forecastSharpness(leadHours: number): string {
  if (leadHours <= 48) {
    return "Within 2 days out, so this uses ICON-D2 — about 2 km resolution.";
  }
  if (leadHours <= 78) {
    return "More than 2 days out: ICON-EU/global guidance, about 7–11 km resolution.";
  }

  return "More than 3 days out: coarser global guidance, past ICON's finer-grained range.";
}

/**
 * The felt temperature first, the dry-bulb one behind it.
 *
 * On a bike at 25 km/h the apparent temperature is the figure that decides
 * what goes in the jersey pocket — wind chill on a descent is most of the
 * difference between the two — so it leads. The reading a thermometer by the
 * road would give still travels beside it rather than instead of it, because
 * the two disagreeing by ten degrees is itself worth seeing.
 */
function temperatureText(point: WeatherPoint, unitSystem: UnitSystem): string {
  const apparent = formatTemperature(point.apparentTemperatureCelsius, unitSystem);
  const actual = formatTemperature(point.temperatureCelsius, unitSystem);

  return `feels ${apparent}, ${actual} actual`;
}

/**
 * What tells one cell from another.
 *
 * Not the distance alone: a stage with repeated coordinates puts several
 * samples at the same point on the ground, and React would be handed the same
 * key twice. The clock always moves even when the road does not.
 */
function cellKey(sample: ForecastSample): string {
  return `${sample.distanceMetres}-${sample.arrivalAt.getTime()}`;
}

function conditionText(point: WeatherPoint, unitSystem: UnitSystem): string {
  const label = weatherCodeLabel(point.weatherCode);
  if (point.precipitationProbabilityPercent <= 0 && point.precipitationMillimetres <= 0) {
    // Said rather than left to the missing fill. A dry cell is drawn by having
    // no shading at all, which is exactly the "carried by colour alone" state
    // this page does not allow — and "overcast" on its own says nothing about
    // whether rain was forecast. A pair of zeros would be the same fact in a
    // form nobody reads out loud.
    return `${label}, no rain expected`;
  }

  return `${label}, ${Math.round(point.precipitationProbabilityPercent)}% chance of ${formatPrecipitation(point.precipitationMillimetres, unitSystem)}`;
}

/**
 * The wind a rider actually feels, with the weather station's reading behind
 * it.
 *
 * A bare "18 km/h from 240°" asks the reader to do trigonometry in their head,
 * which is the whole reason this strip classifies the wind at all. The number
 * that leads is therefore the component along the direction of travel: a
 * crosswind leaning slightly ahead is a couple of kilometres an hour against
 * you, not eighteen. The raw speed and bearing still travel, because they are
 * what a forecast anywhere else will quote back.
 */
function windText(
  point: WeatherPoint,
  relation: CellRelation,
  componentKmhPerKmh: number | null,
  unitSystem: UnitSystem,
): string {
  const speed = formatWindSpeed(point.windSpeedKmh, unitSystem);
  const direction = `from ${compassLabel(point.windDirectionDegrees)} (${Math.round(point.windDirectionDegrees)}°)`;
  const named =
    relation === "head"
      ? "Headwind"
      : relation === "tail"
        ? "Tailwind"
        : relation === "cross"
          ? "Crosswind"
          : relation === "mixed"
            ? "Mixed direction here"
            : null;
  if (named === null) {
    return `${speed} ${direction}`;
  }
  // Mixed means the road turns through the window, so there is no one
  // component to quote: the same wind is ahead and behind within a kilometre.
  if (componentKmhPerKmh === null || relation === "mixed") {
    return `${named}, ${speed} ${direction}`;
  }
  const along = formatWindSpeed(Math.abs(componentKmhPerKmh) * point.windSpeedKmh, unitSystem);
  /*
   * "Headwind" and "Tailwind" already say which way their component pushes, so
   * the magnitude alone is the whole of it. A crosswind is the reading that
   * still leans one way or the other, and the sign is the only thing carrying
   * that: without it, a cross leaning into the rider and one pushing them
   * along announce the same number.
   */
  if (relation === "cross") {
    const lean = componentKmhPerKmh < 0 ? "against you" : "with you";

    return `${named}, ${along} ${lean} along the route, ${speed} ${direction}`;
  }

  return `${named} ${along} along the route, ${speed} ${direction}`;
}

/** The letter a cell's own glyph draws for a wind relation. */
const RELATION_GLYPH: Record<Exclude<CellRelation, null>, string> = {
  head: "H",
  tail: "T",
  cross: "X",
  mixed: "M",
};

export interface ForecastStripProps {
  samples: ForecastSample[];
  coordinates: Position[];
  /** The stretch on show — the same window `ElevationProfile` is drawing. */
  startMetres: number;
  endMetres: number;
  unitSystem: UnitSystem;
}

export function ForecastStrip({
  samples,
  coordinates,
  startMetres,
  endMetres,
  unitSystem,
}: ForecastStripProps) {
  const { ref, width } = useElementWidth<HTMLDivElement>();
  const distances = useMemo(() => cumulativeMetres(coordinates), [coordinates]);
  const totalMetres = distances[distances.length - 1] ?? 0;

  const forecast = useQuery({ ...weatherQuery(samples), enabled: samples.length > 0 });

  const cells = useMemo(() => {
    const points = forecast.data?.points;
    if (!points || points.length !== samples.length) {
      return [];
    }

    return samples.flatMap((sample, index) => {
      const point = points[index];
      if (!point) {
        return [];
      }
      const previous = samples[index - 1];
      const next = samples[index + 1];
      const cellStart = previous ? (previous.distanceMetres + sample.distanceMetres) / 2 : 0;
      const cellEnd = next ? (sample.distanceMetres + next.distanceMetres) / 2 : totalMetres;
      // Outside the stretch the chart is drawing — a zoomed elevation profile
      // narrows the axis this strip shares with it, and a cell wholly outside
      // that window has nothing to draw itself against.
      //
      // A window of no width is not a window that excludes everything: a stage
      // that covers no ground still has a timeline, and dropping every cell
      // against it would take the readable table down with the graphic. There
      // is nothing to clip to, so nothing is clipped.
      const clips = endMetres > startMetres;
      if (clips && (cellEnd <= startMetres || cellStart >= endMetres)) {
        return [];
      }

      const bearing = bearingAt(
        coordinates,
        distances,
        sample.distanceMetres,
        BEARING_WINDOW_METRES,
      );
      const mixed =
        bearing !== null &&
        bearingIsMixed(coordinates, distances, sample.distanceMetres, BEARING_WINDOW_METRES);
      const reading = bearing !== null ? windRelation(bearing, point.windDirectionDegrees) : null;
      const relation: CellRelation = mixed ? "mixed" : (reading?.relation ?? null);

      return [
        {
          sample,
          point,
          cellStart,
          cellEnd,
          relation,
          component: reading?.componentKmhPerKmh ?? null,
        },
      ];
    });
  }, [samples, forecast.data, coordinates, distances, totalMetres, startMetres, endMetres]);

  if (samples.length === 0) {
    return null;
  }
  if (forecast.isError) {
    /*
     * A failure the service owns — a 502 carrying the provider's own outage,
     * or a 5xx of its own — is not the reader's to act on, and reads as the
     * forecast being unavailable. A 4xx is: it is this page having asked for
     * something the endpoint refuses, a window it cannot answer or more points
     * than it takes, and reporting that as an outage would send the reader off
     * to check whether Open-Meteo is down over arithmetic done here. The start
     * time is re-checked before the request is ever made, so this is the belt
     * to that braces.
     */
    const provider = forecast.error instanceof ApiError && forecast.error.status >= 500;

    return (
      <p className="forecast-strip__unavailable">
        {provider
          ? "The forecast is unavailable right now; the rest of this route is unaffected."
          : "This forecast could not be requested for this ride; the rest of this route is unaffected."}
      </p>
    );
  }
  if (!forecast.data || cells.length === 0) {
    return null;
  }

  const { plotWidth, x } = plotAxis(width, startMetres, endMetres);
  // The same total the elevation chart draws its own viewBox at — including
  // the padding reserved down the left for its axis labels, which this strip
  // has none of but still leaves clear, so the two plot areas start at the
  // same pixel.
  const totalWidth = plotWidth + PADDING.left + PADDING.right;
  const firstArrival = samples[0]?.arrivalAt;
  const leadHours = firstArrival
    ? Math.max(0, (firstArrival.getTime() - Date.now()) / 3_600_000)
    : 0;

  return (
    <div className="forecast-strip" ref={ref}>
      <svg
        width="100%"
        height={STRIP_HEIGHT}
        viewBox={`0 0 ${totalWidth} ${STRIP_HEIGHT}`}
        role="img"
        aria-label={`Forecast along the way, ${cells.length} readings`}
      >
        <title>{`Forecast along the way, ${cells.length} readings`}</title>
        <g transform={`translate(${PADDING.left} 0)`}>
          {cells.map(({ sample, point, cellStart, cellEnd, relation, component }) => {
            const left = Math.min(Math.max(x(cellStart), 0), plotWidth);
            const right = Math.min(Math.max(x(cellEnd), 0), plotWidth);
            const cellWidth = Math.max(right - left, 0);
            const opacity =
              (point.precipitationProbabilityPercent / 100) * MAX_PRECIPITATION_OPACITY;
            const time = sample.arrivalAt.toLocaleString(undefined, {
              weekday: "short",
              hour: "2-digit",
              minute: "2-digit",
            });
            const title = `${time} — ${temperatureText(point, unitSystem)} · ${windText(point, relation, component, unitSystem)} · ${conditionText(point, unitSystem)}`;

            return (
              // Distance and arrival together: a stage that stands still —
              // repeated coordinates — gives several samples the same distance,
              // and the clock is what still tells them apart.
              <g key={cellKey(sample)} className="forecast-strip__cell">
                <rect
                  x={left}
                  y={0}
                  width={cellWidth}
                  height={STRIP_HEIGHT}
                  className="forecast-strip__precip"
                  style={{ fillOpacity: opacity }}
                >
                  <title>{title}</title>
                </rect>
                {relation && cellWidth >= MIN_GLYPH_CELL_WIDTH ? (
                  <text
                    className="forecast-strip__wind-glyph"
                    data-relation={relation}
                    x={left + cellWidth / 2}
                    y={STRIP_HEIGHT / 2}
                    textAnchor="middle"
                    dominantBaseline="central"
                  >
                    {RELATION_GLYPH[relation]}
                  </text>
                ) : null}
              </g>
            );
          })}
        </g>
      </svg>

      <p className="forecast-strip__sharpness">{forecastSharpness(leadHours)}</p>
      <p className="forecast-strip__credit">Weather data by Open-Meteo.com</p>

      <table className="visually-hidden">
        <caption>Forecast along the way</caption>
        <thead>
          <tr>
            <th scope="col">Time</th>
            <th scope="col">Temperature</th>
            <th scope="col">Wind</th>
            <th scope="col">Condition</th>
          </tr>
        </thead>
        <tbody>
          {cells.map(({ sample, point, relation, component }) => (
            <tr key={cellKey(sample)}>
              <td>
                {sample.arrivalAt.toLocaleString(undefined, {
                  dateStyle: "medium",
                  timeStyle: "short",
                })}
              </td>
              <td>{temperatureText(point, unitSystem)}</td>
              <td>{windText(point, relation, component, unitSystem)}</td>
              <td>{conditionText(point, unitSystem)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
