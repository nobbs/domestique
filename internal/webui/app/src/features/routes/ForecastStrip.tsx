/**
 * A stage's forecast, as a strip of tiles along the route: one per reading, in
 * the order the ride reaches them.
 *
 * A tile rather than a lane per measure. A rider reads a forecast a moment at a
 * time — *what is it like when I get to the top* — so each reading is a small
 * card carrying its own condition, temperature and wind, and the tiles are as
 * wide as the ground they cover, so their own edges show where the ride slows
 * down. The strip is bordered and rounded as one object, because that is what
 * it is: the tiles divide it, they do not sit in it.
 *
 * Rain tints the tile rather than drawing a bar of its own — a tile already
 * carries three readings, and a fourth mark inside it is the crowding this
 * layout exists to avoid. Wet moments go visibly heavier as a front arrives.
 *
 * Runs its own query — `MapCredits` is this component's own precedent for that
 * — so a caller only has to hand over what the forecast is asked about, not
 * thread a query result through props.
 *
 * The tile's arrow is a compass arrow, north up: it points the way the air is
 * going, which is what the tooltip's arrow and the drifting field on the map
 * both say. Measured against the way the rider faces instead — as it once was
 * — the same wind read one way in a tile and the opposite way on the map
 * wherever the road ran south, with nothing in a row of weather tiles to tell
 * a reader which of the two frames they were looking at. What the wind does
 * *to the rider* is the route's own tint, and it is said in colour there
 * rather than in a second arrow here.
 */

import { IconArrowUp } from "@tabler/icons-react";
import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { weatherQuery } from "../../api/queries";
import { ApiError } from "../../api/request";
import type { Position } from "../../api/types";
import { forecastResolution } from "../../lib/forecastResolution";
import type { ForecastSample } from "../../lib/forecastSamples";
import { forecastLeadHours } from "../../lib/forecastSamples";
import { formatClock, formatWindSpeed } from "../../lib/format";
import { PADDING, plotAxis } from "../../lib/plotAxis";
import { compassPoint } from "../../lib/routeCues";
import { useElementWidth } from "../../lib/useElementWidth";
import { temperatureColour, weatherIcon } from "../../lib/weather";
import { buildCells, windWeight } from "./forecastCells";

const TILE_HEIGHT = 76;

/**
 * Corners on the strip, not on the tiles.
 *
 * Rounding each tile turns the band into a row of loose chips, and a gap
 * between them is ground the strip claims to cover and then does not draw. The
 * clip that puts the corners on the ends is also what shapes the first and last
 * tiles, so neither has to know it is first or last.
 */
const STRIP_RADIUS = "0.375rem";

/*
 * What a tile gives up as it narrows, and in which order.
 *
 * Temperature survives longest because it is the reading a rider acts on; the
 * wind goes first because it changes slowest across a day. The order matters
 * more than the numbers: dropping the figure before the glyph leaves a tile
 * that is decorated but says nothing.
 */
const MIN_WIND_WIDTH = 34;
/** Room for "12 km · 14:20"; narrower tiles keep the clock alone. */
const MIN_PLACE_WIDTH = 72;
const MIN_ICON_WIDTH = 26;
const MIN_FIGURE_WIDTH = 18;

export interface ForecastStripProps {
  samples: ForecastSample[];
  coordinates: Position[];
  /** The stretch on show — the same window `ElevationProfile` is drawing. */
  startMetres: number;
  endMetres: number;
  /** Whether the strip reserves the chart's own left/right gutters. */
  inset?: boolean;
  /** Whether the resolution sentence below the strip is shown. */
  caption?: boolean;
  /** Grows the strip to fill a flex parent's height instead of sitting at a fixed one. */
  fill?: boolean;
}

export function ForecastStrip({
  samples,
  coordinates,
  startMetres,
  endMetres,
  inset = true,
  caption = true,
  fill = false,
}: ForecastStripProps) {
  // Nothing to ask about without samples: the endpoint refuses an empty
  // request, and the strip renders nothing for one anyway.
  const forecast = useQuery({ ...weatherQuery(samples), enabled: samples.length > 0 });
  const { ref, width } = useElementWidth<HTMLDivElement>();
  const cells = useMemo(
    () => (forecast.data ? buildCells(samples, forecast.data.points, coordinates) : []),
    [samples, forecast.data, coordinates],
  );

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
     * to those braces.
     */
    const provider = forecast.error instanceof ApiError && forecast.error.status >= 500;

    return (
      // Announced, not merely rendered: this appears when a request the reader
      // never watched comes back, and every other asynchronous failure in this
      // UI says so out loud.
      <p className="mt-2 text-sm text-[var(--alert)]" role="alert">
        {provider
          ? "The forecast is unavailable right now; the rest of this route is unaffected."
          : "This forecast could not be requested for this ride; the rest of this route is unaffected."}
      </p>
    );
  }
  if (!forecast.data || cells.length === 0) {
    return null;
  }

  const { x } = plotAxis(width, startMetres, endMetres);
  const leadHours = forecastLeadHours(samples);

  return (
    // Named as a group rather than as an image: the old strip was one graphic
    // with a hidden table beside it, and this one is tiles carrying their own
    // readings — marking it `img` would hide every figure on it.
    <div
      ref={ref}
      role="group"
      aria-label={`Forecast along the way, ${cells.length} readings`}
      className={fill ? "flex h-full flex-col" : undefined}
    >
      {/*
       * The chart's own gutters, left clear rather than drawn in: this strip
       * has no axis labels of its own but reserves the same margin, which is
       * what keeps its tiles under the terrain they describe.
       */}
      <div
        className={fill ? "min-h-0 flex-1" : undefined}
        style={{ paddingLeft: inset ? PADDING.left : 0, paddingRight: inset ? PADDING.right : 0 }}
      >
        <div
          className="relative h-full overflow-hidden border border-[var(--rule)]"
          style={
            fill
              ? { borderRadius: STRIP_RADIUS }
              : { height: TILE_HEIGHT, borderRadius: STRIP_RADIUS }
          }
        >
          {cells.map((cell) => {
            const left = x(cell.startMetres);
            const cellWidth = Math.max(x(cell.endMetres) - left, 0);
            const Condition = weatherIcon(cell.point.weatherCode);
            const figures = cellWidth >= MIN_FIGURE_WIDTH;
            const icon = cellWidth >= MIN_ICON_WIDTH;
            const wind = cellWidth >= MIN_WIND_WIDTH;
            const clock = formatClock(cell.sample.arrivalAt);
            const place =
              cellWidth >= MIN_PLACE_WIDTH
                ? `${Math.round(cell.sample.distanceMetres / 1000)} km · ${clock}`
                : wind
                  ? clock
                  : null;
            const wet = (cell.point.precipitationProbabilityPercent / 100) * 0.5;

            return (
              <div
                key={cell.sample.arrivalAt.getTime()}
                className="absolute top-0 flex flex-col items-center justify-center gap-0.5 overflow-hidden border-[var(--rule)] not-last:border-r"
                style={{
                  left,
                  width: cellWidth,
                  ...(fill ? { bottom: 0 } : { height: TILE_HEIGHT }),
                  backgroundColor: `color-mix(in srgb, var(--accent) ${wet * 100}%, transparent)`,
                }}
              >
                {place === null ? null : (
                  <span className="text-[10px] text-[var(--ink-2)] tabular-nums whitespace-nowrap">
                    {place}
                  </span>
                )}
                <span className="text-[var(--ink-2)]">
                  {icon ? <Condition size={15} stroke={1.7} /> : null}
                </span>
                <span
                  className="rounded px-1 text-[11px] font-semibold text-[var(--ink)] tabular-nums"
                  style={{
                    backgroundColor: `color-mix(in srgb, ${temperatureColour(cell.point.temperatureCelsius)} 60%, transparent)`,
                  }}
                >
                  {/*
                   * The reader's own scale, with the unit left off: a tile is
                   * too small for "°C", and a column of them all in the same
                   * scale needs saying once rather than per tile — which the
                   * hidden label below does.
                   */}
                  {figures ? `${Math.round(cell.point.temperatureCelsius)}°` : null}
                </span>
                <span
                  className="flex items-center gap-0.5 text-[10px] text-[var(--ink-2)] tabular-nums"
                  style={{ opacity: 0.4 + windWeight(cell.point.windSpeedKmh) * 0.6 }}
                  // The glyph is a picture and the figure beside it a bare
                  // number, so between them they say nothing about direction to
                  // a reader who is not looking at the strip.
                  aria-label={
                    wind
                      ? `Wind ${formatWindSpeed(cell.point.windSpeedKmh)} toward the ${compassPoint(cell.flowDegrees)}`
                      : undefined
                  }
                >
                  {wind ? (
                    <IconArrowUp
                      size={12}
                      stroke={2.2}
                      style={{ transform: `rotate(${cell.flowDegrees}deg)` }}
                    />
                  ) : null}
                  {wind ? Math.round(cell.point.windSpeedKmh) : null}
                </span>
              </div>
            );
          })}
        </div>
      </div>
      {caption ? (
        <p
          className="mt-1 text-xs text-[var(--ink-2)]"
          style={{ paddingLeft: inset ? PADDING.left : 0 }}
        >
          {forecastResolution(leadHours).sentence}
        </p>
      ) : null}
    </div>
  );
}
