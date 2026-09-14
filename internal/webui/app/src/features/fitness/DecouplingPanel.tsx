/**
 * Aerobic decoupling, one dot per ride, with a rolling four-week median.
 *
 * Dots rather than a line: decoupling is a property of each ride, not a
 * quantity that carries from one to the next. The median is what shows the
 * cloud drifting, which is the signal a reader is after.
 */

import type { Activity } from "../../api/types";
import { ChartLegend, FRAME_LEFT, ReadoutRow, TimeFrame } from "../../components/chart/TimeFrame";
import { formatDuration } from "../../lib/format";
import { signed } from "./form";
import { FITNESS_COLOUR } from "./SeasonChart";

/** Under this, on a steady ride of an hour or more, the aerobic base is sound. */
const SOUND_PERCENT = 5;
const TREND_DAYS = 28;
const COMPARE_DAYS = 56;

export interface DecouplingRide {
  id: string;
  /** The calendar day it started, in the service's zone. */
  date: string;
  percent: number;
  movingSeconds: number;
}

const dayFormatters = new Map<string, Intl.DateTimeFormat>();

/** The calendar day an instant falls on in `zone`, as `YYYY-MM-DD`. */
export function calendarDay(instant: string, zone: string): string {
  let formatter = dayFormatters.get(zone);
  if (!formatter) {
    formatter = new Intl.DateTimeFormat("en-CA", {
      timeZone: zone,
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
    });
    dayFormatters.set(zone, formatter);
  }

  return formatter.format(new Date(instant));
}

/** The rides that carry a decoupling figure; one without it had no meter or was too short. */
export function decouplingRides(activities: readonly Activity[], zone: string): DecouplingRide[] {
  return activities.flatMap((ride) => {
    const percent = ride.metrics?.decouplingPercent;
    if (percent === undefined || !Number.isFinite(Date.parse(ride.startedAt))) {
      return [];
    }

    return [
      {
        id: ride.id,
        date: calendarDay(ride.startedAt, zone),
        percent,
        movingSeconds: ride.movingSeconds,
      },
    ];
  });
}

export function median(values: readonly number[]): number | undefined {
  const sorted = [...values].sort((one, other) => one - other);
  const middle = Math.floor(sorted.length / 2);
  if (sorted.length === 0) {
    return undefined;
  }

  return sorted.length % 2 === 1
    ? sorted[middle]
    : ((sorted[middle - 1] ?? 0) + (sorted[middle] ?? 0)) / 2;
}

/** The chart's percent range: the rides shown and the median, which also reads rides before the range. */
export function decouplingDomain(
  shown: readonly DecouplingRide[],
  trend: readonly (number | undefined)[],
): [number, number] {
  const values = [
    ...shown.map((ride) => ride.percent),
    ...trend.filter((value) => value !== undefined),
  ];

  return [Math.min(-2, ...values), Math.max(12, ...values)];
}

/** Each unbroken stretch of defined values, as index–value pairs, so a gap is never drawn across. */
export function definedRuns(
  values: readonly (number | undefined)[],
): Array<Array<[number, number]>> {
  const runs: Array<Array<[number, number]>> = [];
  let run: Array<[number, number]> = [];
  values.forEach((value, index) => {
    if (value === undefined) {
      if (run.length > 0) {
        runs.push(run);
      }
      run = [];
    } else {
      run.push([index, value]);
    }
  });
  if (run.length > 0) {
    runs.push(run);
  }

  return runs;
}

const daysBefore = (date: string, days: number) => {
  const at = new Date(`${date}T00:00:00Z`);
  at.setUTCDate(at.getUTCDate() - days);

  return at.toISOString().slice(0, 10);
};

interface Props {
  rides: readonly DecouplingRide[];
  dates: readonly string[];
}

/** The median over the rides after `from` up to and including `to`. */
function medianBetween(rides: readonly DecouplingRide[], from: string, to: string) {
  return median(
    rides.filter((ride) => ride.date > from && ride.date <= to).map((ride) => ride.percent),
  );
}

export function DecouplingSummary({ rides, dates }: Props) {
  const today = dates[dates.length - 1];
  if (!today) {
    return null;
  }
  const now = medianBetween(rides, daysBefore(today, COMPARE_DAYS), today);
  const before = medianBetween(
    rides,
    daysBefore(today, COMPARE_DAYS * 2),
    daysBefore(today, COMPARE_DAYS),
  );
  if (now === undefined) {
    return null;
  }

  return (
    <span className="text-[var(--ink-2)] text-xs tabular-nums">
      {now.toFixed(1)}% median, last 8 weeks
      {before === undefined ? "" : ` · ${signed(now - before, 1)} on the 8 before`}
    </span>
  );
}

export function DecouplingPanel({ rides, dates }: Props) {
  const indexOf = new Map(dates.map((date, index) => [date, index]));
  const shown = rides.filter((ride) => indexOf.has(ride.date));
  if (shown.length === 0) {
    return null;
  }
  const trend = dates.map((date) => medianBetween(rides, daysBefore(date, TREND_DAYS), date));
  const [low, high] = decouplingDomain(shown, trend);

  return (
    <>
      <ChartLegend
        items={[
          { label: "Ride", colour: FITNESS_COLOUR, swatch: true },
          { label: "4-week median", colour: "var(--ink)" },
        ]}
      />
      <TimeFrame
        label={`Aerobic decoupling over ${shown.length} ${shown.length === 1 ? "ride" : "rides"}, with a rolling four-week median`}
        dates={dates}
        snap={shown.map((ride) => indexOf.get(ride.date) ?? 0)}
        readout={(index) =>
          shown
            .filter((ride) => indexOf.get(ride.date) === index)
            .map((ride) => (
              <div key={ride.id}>
                <ReadoutRow label="decoupling" value={`${ride.percent.toFixed(1)}%`} />
                <ReadoutRow label="moving" value={formatDuration(ride.movingSeconds)} />
              </div>
            ))
            .concat(
              trend[index] === undefined
                ? []
                : [
                    <ReadoutRow
                      key="trend"
                      colour="var(--ink)"
                      label="4-week median"
                      value={`${trend[index]?.toFixed(1)}%`}
                    />,
                  ],
            )
        }
        panels={[
          {
            height: 150,
            domain: [low, high],
            format: (value) => `${value}%`,
            draw: (x, y) => (
              <>
                <rect
                  x={FRAME_LEFT}
                  y={y(SOUND_PERCENT)}
                  width={x(dates.length - 1) - FRAME_LEFT}
                  height={y(low) - y(SOUND_PERCENT)}
                  fill="var(--good)"
                  opacity={0.08}
                />
                {shown.map((ride) => (
                  <circle
                    key={ride.id}
                    cx={x(indexOf.get(ride.date) ?? 0)}
                    cy={y(ride.percent)}
                    r={3}
                    fill={FITNESS_COLOUR}
                    opacity={0.55}
                  />
                ))}
                {definedRuns(trend).map((run) => (
                  <polyline
                    key={run[0]?.[0]}
                    points={run.map(([index, value]) => `${x(index)},${y(value)}`).join(" ")}
                    fill="none"
                    stroke="var(--ink)"
                    strokeWidth={2}
                  />
                ))}
              </>
            ),
          },
        ]}
      />
      <p className="text-[var(--ink-2)] text-xs">
        Per ride, the share of the first half's power-to-heart-rate ratio lost over the second.
        Measured power only, over rides of an hour or more. Under 5% (shaded) on a steady ride is a
        sound aerobic base.
      </p>
    </>
  );
}
