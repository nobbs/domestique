/**
 * What one ride was actually ridden through, laid under the terrain it was
 * ridden over: the course's forecast strip, read back.
 *
 * One tile per step, as wide as the ground the ride covered in it, so a tile
 * sits under the kilometres it describes and a long climb is visibly the hour
 * it rained. The strip reserves the profile's own gutters through the shared
 * plot axis, which is what keeps its edges on the chart's. No probability of
 * rain: for a ride old enough to be answered by reanalysis there is none, and
 * for one that has already happened it would say nothing anyway.
 */

import { IconArrowUp } from "@tabler/icons-react";
import type { ActivitySplit, RideWeatherStep } from "../../api/types";
import {
  formatClock,
  formatKilometres,
  formatPrecipitation,
  formatWindSpeed,
} from "../../lib/format";
import { PADDING, plotAxis } from "../../lib/plotAxis";
import { compassPoint } from "../../lib/routeCues";
import { useElementWidth } from "../../lib/useElementWidth";
import { temperatureColour, weatherIcon } from "../../lib/weather";
import { flowBearingDegrees } from "../../lib/windField";
import { windWeight } from "../routes/forecastCells";

const TILE_HEIGHT = 68;
/** What a tile gives up as it narrows, in the forecast strip's order. */
const MIN_PLACE_WIDTH = 84;
const MIN_CLOCK_WIDTH = 34;
const MIN_WIND_WIDTH = 34;
const MIN_ICON_WIDTH = 26;
const MIN_FIGURE_WIDTH = 18;

/**
 * Where along the ride each step began, in metres from the start, on the axis
 * `totalMetres` long that the strip is drawn against.
 *
 * Walked along the splits' own clock: the stretch being ridden when a step's
 * moving time arrives is where that step started. A stop shifts everything
 * after it earlier along the ride than it really was, which is the error of
 * having no clock on the track itself. The splits are cut from the odometer
 * and the axis is measured from positions, so the two lengths differ by a
 * little; the walked distance is scaled so the last split ends where the axis
 * does. Without splits the steps are spread by elapsed time instead, which is
 * right for a ride that never slowed down.
 */
export function stepStarts(
  steps: RideWeatherStep[],
  startedAt: string,
  elapsedSeconds: number,
  totalMetres: number,
  splits: ActivitySplit[],
): number[] {
  const started = new Date(startedAt).getTime();
  const odometer = splits.reduce((sum, split) => sum + split.distanceMetres, 0);
  const scale = odometer > 0 ? totalMetres / odometer : 0;

  return steps.map((step) => {
    const seconds = Math.max((new Date(step.time).getTime() - started) / 1000, 0);
    if (splits.length === 0) {
      return elapsedSeconds > 0
        ? Math.min((seconds / elapsedSeconds) * totalMetres, totalMetres)
        : 0;
    }
    let elapsed = 0;
    let covered = 0;
    for (const split of splits) {
      // The step begins somewhere in this stretch, so it starts where the stretch does.
      if (elapsed + split.movingSeconds > seconds) {
        break;
      }
      elapsed += split.movingSeconds;
      covered += split.distanceMetres;
    }

    return Math.min(covered * scale, totalMetres);
  });
}

/**
 * What the arrow and the tint say, for a reader who has neither. A dry step
 * says nothing about rain rather than saying none fell.
 */
function windAndRain(step: RideWeatherStep): string {
  // Toward, not from: the provider says where the wind came from, and every
  // arrow in this application points the way the air is going.
  const wind = `Wind ${formatWindSpeed(step.windSpeedKmh)} toward the ${compassPoint(
    flowBearingDegrees(step.windDirectionDegrees),
  )}`;

  return step.precipitationMillimetres > 0
    ? `${wind}, ${formatPrecipitation(step.precipitationMillimetres)}`
    : wind;
}

export interface RideConditionsProps {
  steps: RideWeatherStep[] | undefined;
  /** Where each step began, from `stepStarts`; one entry per step. */
  starts: number[];
  totalMetres: number;
  /** Whether the strip reserves the chart's own left/right gutters. */
  inset?: boolean;
}

export function RideConditions({ steps, starts, totalMetres, inset = true }: RideConditionsProps) {
  const { ref, width } = useElementWidth<HTMLDivElement>();
  if (!steps || steps.length === 0 || totalMetres <= 0) {
    return null;
  }
  const { x } = plotAxis(width, 0, totalMetres);
  const cells = steps
    .map((step, index) => ({
      step,
      startMetres: starts[index] ?? totalMetres,
      endMetres: starts[index + 1] ?? totalMetres,
    }))
    .filter((cell) => cell.startMetres < totalMetres);
  // Nothing placed yet — the ride's summary still loading — is not a strip
  // with nothing in it.
  if (cells.length === 0) {
    return null;
  }

  return (
    <div
      ref={ref}
      style={{ paddingLeft: inset ? PADDING.left : 0, paddingRight: inset ? PADDING.right : 0 }}
    >
      <ul
        aria-label="Conditions"
        className="relative overflow-hidden rounded-md border border-[var(--rule)]"
        style={{ height: TILE_HEIGHT }}
      >
        {cells.map(({ step, startMetres, endMetres }) => {
          const Glyph = weatherIcon(step.weatherCode);
          const at = new Date(step.time);
          const left = x(startMetres);
          const cellWidth = Math.max(x(endMetres) - left, 0);
          const place =
            cellWidth >= MIN_PLACE_WIDTH
              ? `${formatKilometres(startMetres)} · ${formatClock(at)}`
              : cellWidth >= MIN_CLOCK_WIDTH
                ? formatClock(at)
                : null;
          const wind = cellWidth >= MIN_WIND_WIDTH;
          const wet = Math.min(step.precipitationMillimetres / 5, 1) * 0.5;

          return (
            <li
              key={step.time}
              className="absolute top-0 flex h-full flex-col items-center justify-center gap-0.5 overflow-hidden border-[var(--rule)] not-last:border-r"
              style={{
                left,
                width: cellWidth,
                backgroundColor: `color-mix(in srgb, var(--rain-1) ${wet * 100}%, transparent)`,
              }}
            >
              {place === null ? null : (
                <span className="text-[10px] text-[var(--ink-2)] tabular-nums whitespace-nowrap">
                  {place}
                </span>
              )}
              <span className="flex items-center gap-1.5">
                {cellWidth >= MIN_ICON_WIDTH ? (
                  <Glyph
                    size={15}
                    stroke={1.7}
                    aria-hidden="true"
                    className="text-[var(--ink-2)]"
                  />
                ) : null}
                {cellWidth >= MIN_FIGURE_WIDTH ? (
                  <span
                    className="rounded px-1 font-semibold text-[11px] tabular-nums"
                    style={{
                      backgroundColor: `color-mix(in srgb, ${temperatureColour(step.temperatureCelsius)} 60%, transparent)`,
                    }}
                  >
                    {Math.round(step.temperatureCelsius)}°
                  </span>
                ) : null}
              </span>
              <span
                aria-hidden="true"
                className="flex items-center gap-0.5 text-[10px] text-[var(--ink-2)] tabular-nums"
                style={{ opacity: 0.4 + windWeight(step.windSpeedKmh) * 0.6 }}
              >
                {wind ? (
                  <IconArrowUp
                    size={12}
                    stroke={2.2}
                    style={{
                      transform: `rotate(${flowBearingDegrees(step.windDirectionDegrees)}deg)`,
                    }}
                  />
                ) : null}
                {wind ? Math.round(step.windSpeedKmh) : null}
                {wind && step.precipitationMillimetres > 0
                  ? ` · ${formatPrecipitation(step.precipitationMillimetres)}`
                  : null}
              </span>
              {/* One string, not several nodes: the arrow says nothing to a reader. */}
              <span className="sr-only">{windAndRain(step)}</span>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
