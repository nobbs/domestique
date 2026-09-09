/**
 * The sensor series a ride recorded, drawn over its elevation profile.
 *
 * Nothing is fetched until the rider asks for it. A ride can hold twenty
 * thousand samples, so five series fetched to draw none of them would cost the
 * page more than the track it is already showing; and most bicycles carry no
 * power meter, so most of what could be asked for is not there to draw.
 */

import { useQueries } from "@tanstack/react-query";
import { useMemo } from "react";
import { activitySeriesQuery } from "../../api/queries";
import { ApiError } from "../../api/request";
import type { ActivitySeriesName, Position } from "../../api/types";
import type { Profile } from "../../lib/profile";
import { type AlignedSeries, alignSeries } from "../../lib/rideSeries";

/**
 * The series a ride can answer, in the order the chips read.
 *
 * Ordered by how often a ride actually carries one: heart rate and cadence are
 * on most rides, a power meter on almost none.
 */
export const RIDE_SERIES = [
  { key: "heartRate", label: "Heart rate", unit: "bpm", decimals: 0 },
  { key: "cadence", label: "Cadence", unit: "rpm", decimals: 0 },
  { key: "speed", label: "Speed", unit: "km/h", decimals: 1 },
  { key: "temperature", label: "Temperature", unit: "°C", decimals: 0 },
  { key: "power", label: "Power", unit: "W", decimals: 0 },
] as const satisfies readonly {
  key: ActivitySeriesName;
  label: string;
  unit: string;
  decimals: number;
}[];

/** Its own token per series: five lines on one chart are a categorical encoding. */
const SERIES_COLOURS: Record<ActivitySeriesName, string> = {
  heartRate: "var(--series-heart-rate)",
  cadence: "var(--series-cadence)",
  speed: "var(--series-speed)",
  temperature: "var(--series-temperature)",
  power: "var(--series-power)",
};

/**
 * Where one series has got to, which is what its chip says out loud.
 *
 * `absent` and `unavailable` are kept apart deliberately: the ride recorded no
 * such series is an answer about the ride, and the service could not be asked
 * is an answer about right now. A chip that said "not recorded" for a failed
 * request would tell the rider something false about their own bicycle.
 */
export type SeriesState = "off" | "loading" | "drawn" | "absent" | "unavailable";

export interface RideSeriesResult {
  drawn: AlignedSeries[];
  states: Record<ActivitySeriesName, SeriesState>;
}

/**
 * The series the rider asked for, sampled onto the profile's own axis.
 *
 * A ride that recorded none of a series answers not found, which arrives here
 * as `absent` — a chip that stays off and says why, rather than an error the
 * page has to apologise for.
 */
export function useRideSeries(
  id: number | null,
  shown: ReadonlySet<ActivitySeriesName>,
  coordinates: Position[],
  profile: Profile | null,
): RideSeriesResult {
  const results = useQueries({
    queries: RIDE_SERIES.map((series) => ({
      ...activitySeriesQuery(id ?? 0, series.key),
      enabled: id !== null && shown.has(series.key),
    })),
  });

  return useMemo(() => {
    const drawn: AlignedSeries[] = [];
    const states = {} as Record<ActivitySeriesName, SeriesState>;
    RIDE_SERIES.forEach((series, index) => {
      const result = results[index];
      if (!shown.has(series.key)) {
        states[series.key] = "off";

        return;
      }
      if (result?.isError) {
        const notFound = result.error instanceof ApiError && result.error.isNotFound;
        states[series.key] = notFound ? "absent" : "unavailable";

        return;
      }
      const values = result?.data?.values;
      if (!values || !profile) {
        states[series.key] = "loading";

        return;
      }
      states[series.key] = "drawn";
      drawn.push({
        key: series.key,
        label: series.label,
        unit: series.unit,
        decimals: series.decimals,
        colour: SERIES_COLOURS[series.key],
        values: alignSeries(values, coordinates, profile),
      });
    });

    return { drawn, states };
    // The query results are read by index; their identities are what changes.
  }, [results, shown, coordinates, profile]);
}

export interface SeriesChipsProps {
  states: Record<ActivitySeriesName, SeriesState>;
  drawn: AlignedSeries[];
  /** Which sample the shared cursor is on, or null when nothing is hovered. */
  activeIndex: number | null;
  onToggle: (series: ActivitySeriesName) => void;
}

/**
 * One chip per series: what it is, in its own colour, and — with the shared
 * cursor somewhere on the ride — what it read there.
 *
 * The reading rides in the chip rather than in the chart's own footer, which
 * belongs to the profile and is shared with the route pages.
 */
export function SeriesChips({ states, drawn, activeIndex, onToggle }: SeriesChipsProps) {
  return (
    <div className="flex flex-wrap gap-1.5">
      {RIDE_SERIES.map((series) => {
        const state = states[series.key];
        const drawing = drawn.find((one) => one.key === series.key);
        const value = drawing && activeIndex !== null ? drawing.values[activeIndex] : null;

        return (
          <button
            key={series.key}
            type="button"
            onClick={() => onToggle(series.key)}
            // What the rider asked for, whatever came of it: a chip whose
            // series turned out absent must not read as one never pressed,
            // since pressing it again is what puts it away.
            aria-pressed={state !== "off"}
            className="flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs ring-1 ring-[var(--rule)] aria-pressed:bg-[var(--ground)] aria-pressed:ring-[var(--ink-2)]"
          >
            {/*
             * Filled while the series is drawn, hollow while it is not: the
             * chip has to say which it is with no cursor anywhere on the ride,
             * and a background tint alone is too quiet to carry that.
             */}
            <span
              aria-hidden="true"
              className="h-2 w-2 rounded-full border-2"
              // The chip's colour is which series it is, so it is the same
              // information the line carries and is exempted with it.
              style={{
                borderColor: SERIES_COLOURS[series.key],
                background: drawing ? SERIES_COLOURS[series.key] : "transparent",
                forcedColorAdjust: "none",
              }}
            />
            <span>{series.label}</span>
            <span className="text-[var(--ink-2)] tabular-nums">
              {chipReading(state, value, series)}
            </span>
          </button>
        );
      })}
    </div>
  );
}

/**
 * What a chip says after its name: the reading under the cursor, or the state
 * that explains why there is none.
 */
function chipReading(
  state: SeriesState,
  value: number | null | undefined,
  series: (typeof RIDE_SERIES)[number],
): string {
  if (state === "absent") {
    return "not recorded";
  }
  if (state === "unavailable") {
    return "unavailable";
  }
  if (state === "loading") {
    return "…";
  }
  if (state === "off" || value === null || value === undefined) {
    return "";
  }

  return `${value.toFixed(series.decimals)} ${series.unit}`;
}
