/**
 * **D — Cards.** The control: what every weather app does.
 *
 * Deliberately *not* pinned to the distance axis. Each reading gets the same
 * width whether it covers four kilometres of hairpins or eighteen of descent,
 * and the row scrolls sideways when the day is longer than the card is wide.
 * That buys room — the time, both temperatures, the rain figure and the wind
 * speed all fit, with nothing dropped and nothing needing a hover.
 *
 * It is here to settle whether the shared axis earns its cost. Everything the
 * other three give up space for, this one has; what it cannot do is tell you
 * that the headwind starts at the top of the climb, because it no longer
 * knows where the climb is.
 */

import { IconArrowUp, IconWind } from "@tabler/icons-react";
import { formatTemperature, formatWindSpeed } from "../../../lib/format";
import { temperatureColour, weatherIcon } from "../../../lib/weather";
import { windWeight } from "./cells";
import type { BandProps } from "./LanesBand";

const CARD_WIDTH = 74;

export function CardsBand({ cells, unitSystem }: BandProps) {
  return (
    <div className="flex gap-1 overflow-x-auto pb-1">
      {cells.map((cell) => {
        const Condition = weatherIcon(cell.point.weatherCode);
        const wet = cell.point.precipitationProbabilityPercent;

        return (
          <div
            key={cell.sample.arrivalAt.getTime()}
            className="flex shrink-0 flex-col items-center gap-1 rounded border border-[var(--rule)] bg-[var(--panel)] px-1 py-1.5"
            style={{ width: CARD_WIDTH }}
          >
            <span className="text-[10px] text-[var(--ink-2)] tabular-nums">
              {cell.sample.arrivalAt.toLocaleTimeString(undefined, {
                hour: "2-digit",
                minute: "2-digit",
              })}
            </span>
            <span className="text-[var(--ink-2)]">
              <Condition size={18} stroke={1.7} />
            </span>
            <span
              className="w-full rounded text-center font-semibold text-[12px] text-[var(--ink)] tabular-nums"
              style={{
                backgroundColor: `color-mix(in srgb, ${temperatureColour(cell.point.temperatureCelsius)} 60%, transparent)`,
              }}
            >
              {formatTemperature(cell.point.temperatureCelsius, unitSystem)}
            </span>
            {/*
             * The felt temperature, which the axis-pinned layouts have no room
             * for at all. On a descent off a col it is most of what a rider
             * would have wanted to know.
             */}
            <span className="text-[10px] text-[var(--ink-2)] tabular-nums">
              feels {formatTemperature(cell.point.apparentTemperatureCelsius, unitSystem)}
            </span>
            <span
              className="text-[10px] tabular-nums"
              style={{ color: wet >= 50 ? "var(--accent)" : "var(--ink-2)" }}
            >
              {Math.round(wet)}%
            </span>
            <span
              className="flex items-center gap-0.5 text-[10px] text-[var(--ink-2)] tabular-nums"
              style={{ opacity: 0.45 + windWeight(cell.point.windSpeedKmh) * 0.55 }}
            >
              {cell.relation === "mixed" || cell.pushDegrees === null ? (
                <IconWind size={12} stroke={1.8} />
              ) : (
                <IconArrowUp
                  size={12}
                  stroke={2.2}
                  style={{ transform: `rotate(${cell.pushDegrees}deg)` }}
                />
              )}
              {formatWindSpeed(cell.point.windSpeedKmh, unitSystem)}
            </span>
          </div>
        );
      })}
    </div>
  );
}
