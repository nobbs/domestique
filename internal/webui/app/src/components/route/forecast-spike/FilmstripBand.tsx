/**
 * **B — Filmstrip.** One tile per reading, still nailed to the axis.
 *
 * Where **A** asks the reader to scan a lane, this asks them to read a tile:
 * each moment is a ruled cell carrying its own condition, temperature and
 * wind, and the cells are as wide as the ground they cover, so the rules
 * themselves show where the ride slows down. The strip is bordered and
 * rounded as one object, because that is what it is — the tiles divide it,
 * they do not sit in it.
 *
 * The bet: a rider reads a forecast a moment at a time — "what is it like
 * when I get to the top" — and grouping by moment beats grouping by measure.
 * The cost is that comparing one measure across the day means jumping between
 * tiles, which is exactly what **A** is good at.
 */

import { IconArrowUp, IconWind } from "@tabler/icons-react";
import { PADDING, plotAxis } from "../../../lib/plotAxis";
import { temperatureColour, weatherIcon } from "../../../lib/weather";
import { windWeight } from "./cells";
import type { BandProps } from "./LanesBand";

const TILE_HEIGHT = 62;

/**
 * Corners on the strip, not on the tiles.
 *
 * Rounding each tile turned the band into a row of loose chips, and a gap
 * between them is ground the strip is claiming to cover and then not drawing.
 * The band is one object — a continuous reading of the whole ride — so the
 * corners belong to its ends, and the clip that puts them there is also what
 * shapes the first and last tiles without either needing to know it is first
 * or last.
 */
const STRIP_RADIUS = "0.375rem";

/*
 * What a tile gives up as it narrows, and in which order.
 *
 * Temperature survives longest because it is the reading a rider acts on; the
 * wind goes first because it changes slowest across a day and the lane below
 * would still carry it in a layout that had one. The order matters more than
 * the numbers: dropping the figure before the glyph — which is what this did
 * at first — leaves a tile that is decorated but says nothing.
 */
const MIN_WIND_WIDTH = 34;
const MIN_ICON_WIDTH = 26;
const MIN_FIGURE_WIDTH = 18;

export function FilmstripBand({ cells, width, startMetres, endMetres }: BandProps) {
  const { x } = plotAxis(width, startMetres, endMetres);

  return (
    <div style={{ paddingLeft: PADDING.left, paddingRight: PADDING.right }}>
      <div
        className="relative overflow-hidden border border-[var(--rule)]"
        style={{ height: TILE_HEIGHT, borderRadius: STRIP_RADIUS }}
      >
        {cells.map((cell) => {
          const left = x(cell.startMetres);
          const cellWidth = Math.max(x(cell.endMetres) - left, 0);
          const Condition = weatherIcon(cell.point.weatherCode);
          const figures = cellWidth >= MIN_FIGURE_WIDTH;
          const icon = cellWidth >= MIN_ICON_WIDTH;
          const wind = cellWidth >= MIN_WIND_WIDTH;
          /*
           * Rain tints the tile rather than drawing a bar of its own: a tile
           * is already carrying three readings, and a fourth mark inside it
           * is the crowding this layout exists to avoid. Wet moments go
           * visibly heavier as the front arrives.
           */
          const wet = (cell.point.precipitationProbabilityPercent / 100) * 0.5;

          return (
            <div
              key={cell.sample.arrivalAt.getTime()}
              className="absolute top-0 flex flex-col items-center justify-center gap-0.5 overflow-hidden border-[var(--rule)] not-last:border-r"
              style={{
                left,
                width: cellWidth,
                height: TILE_HEIGHT,
                backgroundColor: `color-mix(in srgb, var(--accent) ${wet * 100}%, transparent)`,
              }}
            >
              <span className="text-[var(--ink-2)]">
                {icon ? <Condition size={15} stroke={1.7} /> : null}
              </span>
              <span
                className="rounded px-1 font-semibold text-[11px] text-[var(--ink)] tabular-nums"
                style={{
                  backgroundColor: `color-mix(in srgb, ${temperatureColour(cell.point.temperatureCelsius)} 60%, transparent)`,
                }}
              >
                {figures ? `${Math.round(cell.point.temperatureCelsius)}°` : null}
              </span>
              <span
                className="flex items-center gap-0.5 text-[10px] text-[var(--ink-2)] tabular-nums"
                style={{ opacity: 0.4 + windWeight(cell.point.windSpeedKmh) * 0.6 }}
              >
                {!wind ? null : cell.relation === "mixed" || cell.pushDegrees === null ? (
                  <IconWind size={12} stroke={1.8} />
                ) : (
                  <IconArrowUp
                    size={12}
                    stroke={2.2}
                    style={{ transform: `rotate(${cell.pushDegrees}deg)` }}
                  />
                )}
                {wind ? Math.round(cell.point.windSpeedKmh) : null}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
