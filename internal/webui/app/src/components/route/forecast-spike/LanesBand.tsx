/**
 * **A — Lanes.** The strip's own idea, given room.
 *
 * Everything stays pinned to the distance axis and nothing is dropped; the
 * band simply stops trying to say four things in twenty-two pixels and gives
 * each its own lane. Condition on top, temperature through the middle as the
 * one filled band, wind below it, rain as a footing that grows out of the
 * baseline.
 *
 * The bet: a reader scans *down* a moment to know what it is like there, and
 * *across* a lane to know when it changes — and the second scan is the one
 * the old strip could not support at all, because one lane held two channels.
 */

import { IconArrowUp, IconWind } from "@tabler/icons-react";
import { PADDING, plotAxis } from "../../../lib/plotAxis";
import type { UnitSystem } from "../../../lib/units";
import { temperatureColour, weatherIcon } from "../../../lib/weather";
import type { Cell } from "./cells";
import { windWeight } from "./cells";

const CONDITION_HEIGHT = 20;
const TEMPERATURE_HEIGHT = 26;
const WIND_HEIGHT = 18;
const RAIN_HEIGHT = 12;

/** Below this a cell shows its lane's colour and glyph but no figure. */
const MIN_FIGURE_WIDTH = 24;
/** Below this even the glyph is dropped rather than overlapped with its neighbour. */
const MIN_GLYPH_WIDTH = 15;

export interface BandProps {
  cells: Cell[];
  /** The measured width of the card the band sits in. */
  width: number;
  startMetres: number;
  endMetres: number;
  unitSystem: UnitSystem;
}

export function LanesBand({ cells, width, startMetres, endMetres }: BandProps) {
  const { plotWidth, x } = plotAxis(width, startMetres, endMetres);

  return (
    <div style={{ paddingLeft: PADDING.left, paddingRight: PADDING.right }}>
      <div
        className="relative"
        style={{ height: CONDITION_HEIGHT + TEMPERATURE_HEIGHT + WIND_HEIGHT + RAIN_HEIGHT }}
      >
        {cells.map((cell) => {
          const left = x(cell.startMetres);
          const cellWidth = Math.max(x(cell.endMetres) - left, 0);
          const Condition = weatherIcon(cell.point.weatherCode);
          const figures = cellWidth >= MIN_FIGURE_WIDTH;
          const glyphs = cellWidth >= MIN_GLYPH_WIDTH;

          return (
            <div
              key={cell.sample.arrivalAt.getTime()}
              className="absolute top-0 flex flex-col items-center"
              style={{ left, width: cellWidth }}
            >
              <span
                className="flex items-center justify-center text-[var(--ink-2)]"
                style={{ height: CONDITION_HEIGHT }}
              >
                {glyphs ? <Condition size={16} stroke={1.7} /> : null}
              </span>
              {/*
               * The one filled lane. Temperature is the reading a rider acts
               * on before they leave the house, so it gets the channel that
               * survives being looked at rather than read — and the figure
               * sits inside its own colour rather than beside it.
               */}
              <span
                className="flex w-full items-center justify-center font-medium text-[11px] text-[var(--ink)] tabular-nums"
                style={{
                  height: TEMPERATURE_HEIGHT,
                  backgroundColor: `color-mix(in srgb, ${temperatureColour(cell.point.temperatureCelsius)} 55%, transparent)`,
                }}
              >
                {figures ? `${Math.round(cell.point.temperatureCelsius)}°` : null}
              </span>
              <span
                className="flex items-center justify-center text-[var(--ink-2)]"
                style={{
                  height: WIND_HEIGHT,
                  opacity: 0.35 + windWeight(cell.point.windSpeedKmh) * 0.65,
                }}
              >
                {!glyphs ? null : cell.relation === "mixed" || cell.pushDegrees === null ? (
                  <IconWind size={14} stroke={1.8} />
                ) : (
                  <IconArrowUp
                    size={14}
                    stroke={2.2}
                    style={{ transform: `rotate(${cell.pushDegrees}deg)` }}
                  />
                )}
              </span>
              {/*
               * Rain grows off the baseline rather than filling its lane, so
               * a dry morning is visibly empty. The old strip's absence of
               * colour said the same thing, and it is still the honest
               * drawing of nothing.
               */}
              <span
                className="flex w-full items-end"
                style={{ height: RAIN_HEIGHT }}
                title={`${Math.round(cell.point.precipitationProbabilityPercent)}%`}
              >
                <span
                  className="w-full bg-[var(--accent)]"
                  style={{
                    height: (cell.point.precipitationProbabilityPercent / 100) * RAIN_HEIGHT,
                    opacity: 0.8,
                  }}
                />
              </span>
            </div>
          );
        })}
        <span
          className="absolute bottom-0 block border-[var(--rule)] border-b"
          style={{ left: 0, width: plotWidth }}
        />
      </div>
    </div>
  );
}
