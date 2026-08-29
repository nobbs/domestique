/**
 * **C — Curves.** The day as a shape rather than as a row of moments.
 *
 * The only alternative here that draws what happens *between* samples. A
 * temperature line, filled underneath with the same ramp the other layouts
 * paint their cells from, over columns of rain probability — so a front reads
 * as a slope and a wall rather than as four cells that happen to be darker
 * than the four before them.
 *
 * The bet: what a rider replans around is a *change* — when it turns, how
 * fast, how far it falls — and a change is a thing a curve shows and a grid
 * of cells only implies. The cost is that no single moment is legible: there
 * is no "22° at the top of the col" anywhere on this, only a line passing
 * through where that would be.
 */

import { PADDING, plotAxis } from "../../../lib/plotAxis";
import { temperatureColour } from "../../../lib/weather";
import { windWeight } from "./cells";
import type { BandProps } from "./LanesBand";

const WIND_HEIGHT = 18;
const PLOT_HEIGHT = 74;
const HEIGHT = WIND_HEIGHT + PLOT_HEIGHT;

/** Room above the warmest reading and below the coldest, so neither sits on an edge. */
const HEADROOM_DEGREES = 2;

/** Roughly how far apart the wind arrows are allowed to get, in pixels. */
const WIND_ARROW_SPACING = 44;

/*
 * Drawn rather than borrowed. A Tabler icon is its own `<svg>`, and nesting
 * one inside this chart's `<svg>` renders nothing at all — the cell layouts
 * can use the icon components because they are plain HTML, and this cannot.
 * Pointing up is the wind behind the rider, since the whole angle is measured
 * against the way they face.
 */
const ARROW_PATH = "M0 -6 L0 6 M0 -6 L-3.2 -2 M0 -6 L3.2 -2";
/** No arrow where the road turns through the window: the wind is doing both. */
const MIXED_PATH = "M-6 -1 q3 -4 6 0 q3 4 6 0";

export function CurvesBand({ cells, width, startMetres, endMetres }: BandProps) {
  const { plotWidth, x } = plotAxis(width, startMetres, endMetres);
  const temperatures = cells.map((cell) => cell.point.temperatureCelsius);
  const coldest = Math.min(...temperatures) - HEADROOM_DEGREES;
  const warmest = Math.max(...temperatures) + HEADROOM_DEGREES;
  const span = Math.max(warmest - coldest, 1);
  const y = (celsius: number) =>
    WIND_HEIGHT + PLOT_HEIGHT - ((celsius - coldest) / span) * PLOT_HEIGHT;

  // Plotted at each reading's own position rather than at its cell's middle:
  // this is the one layout that claims to draw a continuous quantity, and a
  // line through the cell centres would be a line through the wrong places.
  const points = cells.map(
    (cell) => `${x(cell.sample.distanceMetres)},${y(cell.point.temperatureCelsius)}`,
  );
  const first = cells[0];
  const last = cells[cells.length - 1];
  const area =
    first && last
      ? `${x(first.sample.distanceMetres)},${HEIGHT} ${points.join(" ")} ${x(last.sample.distanceMetres)},${HEIGHT}`
      : "";

  // One arrow every so many pixels rather than one per reading: at two dozen
  // samples on a card this wide they would touch, and the wind is the reading
  // that changes most slowly.
  let lastArrowX = Number.NEGATIVE_INFINITY;

  return (
    <svg
      width="100%"
      className="block"
      height={HEIGHT}
      viewBox={`0 0 ${plotWidth + PADDING.left + PADDING.right} ${HEIGHT}`}
      role="img"
      aria-label={`Forecast along the way, ${cells.length} readings`}
    >
      <title>{`Forecast along the way, ${cells.length} readings`}</title>
      <defs>
        {/*
         * The ramp itself, laid along the route: a stop at every reading, in
         * that reading's own band colour. It is the same palette the cell
         * layouts fill with, so the two are comparable — this one just draws
         * it as a continuous wash instead of two dozen blocks.
         */}
        <linearGradient id="spike-temperature" x1="0" x2="1" y1="0" y2="0">
          {cells.map((cell) => (
            <stop
              key={cell.sample.arrivalAt.getTime()}
              offset={plotWidth > 0 ? x(cell.sample.distanceMetres) / plotWidth : 0}
              stopColor={temperatureColour(cell.point.temperatureCelsius)}
            />
          ))}
        </linearGradient>
      </defs>
      <g transform={`translate(${PADDING.left} 0)`}>
        {/*
         * Rain over the temperature wash rather than under it. Drawn first,
         * the columns were hidden behind the fill everywhere the day was warm
         * — which is most of the day, and exactly where a rider wants to know
         * it is about to rain anyway.
         */}
        <polygon points={area} fill="url(#spike-temperature)" fillOpacity={0.45} />
        <polyline
          points={points.join(" ")}
          fill="none"
          className="stroke-[var(--ink)]"
          strokeWidth={1.6}
          strokeLinejoin="round"
        />
        {cells.map((cell) => {
          const left = x(cell.startMetres);
          const cellWidth = Math.max(x(cell.endMetres) - left, 0);
          const height = (cell.point.precipitationProbabilityPercent / 100) * PLOT_HEIGHT;

          return (
            <rect
              key={cell.sample.arrivalAt.getTime()}
              x={left}
              y={HEIGHT - height}
              width={cellWidth}
              height={height}
              className="fill-[var(--accent)]"
              style={{ fillOpacity: 0.3 }}
            />
          );
        })}
        {cells.flatMap((cell) => {
          const at = x(cell.sample.distanceMetres);
          if (at - lastArrowX < WIND_ARROW_SPACING) {
            return [];
          }
          lastArrowX = at;

          return [
            <g
              key={cell.sample.arrivalAt.getTime()}
              className="stroke-[var(--ink-2)]"
              fill="none"
              strokeWidth={1.5}
              strokeLinecap="round"
              opacity={0.35 + windWeight(cell.point.windSpeedKmh) * 0.65}
              transform={
                cell.pushDegrees === null || cell.relation === "mixed"
                  ? `translate(${at} 9)`
                  : `translate(${at} 9) rotate(${cell.pushDegrees})`
              }
            >
              <path
                d={cell.relation === "mixed" || cell.pushDegrees === null ? MIXED_PATH : ARROW_PATH}
              />
            </g>,
          ];
        })}
      </g>
    </svg>
  );
}
