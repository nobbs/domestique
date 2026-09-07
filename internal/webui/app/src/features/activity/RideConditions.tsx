/**
 * What one ride was actually ridden through, as a strip of tiles along it: one
 * per step, in the order the ride passed through them.
 *
 * The same shape as a course's forecast strip and for the same reason — a rider
 * reads weather a moment at a time — but by the step rather than by the
 * kilometre, because that is what a provider answers about the past in, and
 * with no probability of rain: for a ride old enough to be answered by
 * reanalysis there is none, and for one that has already happened it would say
 * nothing anyway.
 */

import { IconArrowUp } from "@tabler/icons-react";
import type { RideWeatherStep } from "../../api/types";
import { formatClock, formatPrecipitation, formatWindSpeed } from "../../lib/format";
import { compassPoint } from "../../lib/routeCues";
import { temperatureColour, weatherIcon } from "../../lib/weather";
import { flowBearingDegrees } from "../../lib/windField";

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

/** One step's tile: what it was like, how warm, and what the wind did. */
function StepTile({ step }: { step: RideWeatherStep }) {
  const Glyph = weatherIcon(step.weatherCode);
  const at = new Date(step.time);

  return (
    <li className="flex min-w-20 flex-1 flex-col items-center gap-1 px-2 py-2">
      <span className="text-[var(--ink-2)] text-xs tabular-nums">{formatClock(at)}</span>
      <Glyph aria-hidden="true" size={20} stroke={1.6} />
      <span
        className="font-medium text-sm tabular-nums"
        style={{ color: temperatureColour(step.temperatureCelsius) }}
      >
        {Math.round(step.temperatureCelsius)}°
      </span>
      <span className="flex items-center gap-0.5 text-[var(--ink-2)] text-xs">
        {/*
         * A compass arrow, north up, pointing the way the air is going — the
         * same frame the course strip and the map's field both use.
         */}
        <IconArrowUp
          aria-hidden="true"
          size={12}
          stroke={1.8}
          style={{ transform: `rotate(${flowBearingDegrees(step.windDirectionDegrees)}deg)` }}
        />
        <span className="tabular-nums">{formatWindSpeed(step.windSpeedKmh)}</span>
      </span>
      {/* One string, not several nodes: the arrow says nothing to a reader. */}
      <span className="sr-only">{windAndRain(step)}</span>
    </li>
  );
}

export function RideConditions({ steps }: { steps: RideWeatherStep[] | undefined }) {
  if (!steps || steps.length === 0) {
    return null;
  }

  return (
    <section
      className="rounded-xl bg-[var(--panel)] p-3 ring-1 ring-black/5"
      aria-label="Conditions"
    >
      <h2 className="mb-2 font-medium text-sm">Conditions</h2>
      <ul className="flex overflow-x-auto divide-x divide-[var(--rule)]">
        {steps.map((step) => (
          <StepTile key={step.time} step={step} />
        ))}
      </ul>
    </section>
  );
}
