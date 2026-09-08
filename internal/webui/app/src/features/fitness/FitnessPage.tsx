/**
 * Fitness: what the rider's training has accumulated into, and what it has cost.
 *
 * Volume answers how much has been ridden; this answers what it did to the
 * rider. Both scales are kept because neither converts to the other — TRIMP
 * from heart rate, TSS from power where a meter recorded it and from heart rate
 * where none did — so the toggle picks which one is being read rather than
 * converting between them.
 */

import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { Link } from "react-router";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { activitiesQuery, fitnessQuery } from "../../api/queries";
import type { FitnessWeek } from "../../api/types";
import { PageShell } from "../../components/Layout";
import { Skeleton } from "../../components/ui/skeleton";
import { DecouplingChart, decouplingPoints } from "./DecouplingChart";
import { FitnessChart, type Scale } from "./FitnessChart";
import { formatCurveDuration, PowerCurveChart } from "./PowerCurveChart";

const SCALES: ReadonlyArray<{ value: Scale; label: string }> = [
  { value: "tss", label: "Stress score" },
  { value: "trimp", label: "TRIMP" },
];

/** How far back the page opens, and what the range control offers. */
const RANGES: ReadonlyArray<{ value: string; label: string; days: number }> = [
  { value: "90", label: "3 months", days: 90 },
  { value: "180", label: "6 months", days: 180 },
  { value: "365", label: "1 year", days: 365 },
];

const ZONE_NAMES = ["Recovery", "Endurance", "Tempo", "Threshold", "VO₂ max"];

/** The zones of one week, each bar as wide as its share of the week's riding. */
function WeekBar({ week, widest }: { week: FitnessWeek; widest: number }) {
  const total = week.zoneSeconds.reduce((sum, seconds) => sum + seconds, 0);

  return (
    <li className="grid grid-cols-[5.5rem_1fr] items-center gap-3">
      <span className="text-[var(--ink-2)] text-sm tabular-nums">{week.weekStart}</span>
      <div className="flex flex-col gap-1">
        <div
          aria-hidden="true"
          className="flex h-2 overflow-hidden rounded-full"
          style={{ width: `${(total / widest) * 100}%` }}
        >
          {week.zoneSeconds.map((seconds, zone) => (
            <div
              key={ZONE_NAMES[zone]}
              className="h-full"
              style={{
                width: `${total > 0 ? (seconds / total) * 100 : 0}%`,
                backgroundColor: `color-mix(in oklab, var(--accent) ${20 + zone * 20}%, var(--panel))`,
              }}
            />
          ))}
        </div>
        <span className="text-[var(--ink-2)] text-xs">
          {Math.round(total / 60).toLocaleString()} min
        </span>
      </div>
    </li>
  );
}

export function FitnessPage() {
  const [scale, setScale] = useState<Scale>("tss");
  const [range, setRange] = useState("90");
  const days = RANGES.find(({ value }) => value === range)?.days ?? 90;
  const from = useMemo(() => {
    const start = new Date();
    start.setDate(start.getDate() - days);

    return start.toISOString();
  }, [days]);
  const { data, isPending, isError } = useQuery(fitnessQuery({ from }));
  // The season's decoupling is read off the rides the activities list already
  // carries, rather than asking the service to fold it a second way.
  const activities = useQuery(activitiesQuery());
  const decoupling = useMemo(
    () => decouplingPoints(activities.data ?? [], new Date(from)),
    [activities.data, from],
  );
  const widest = Math.max(
    ...(data?.weeks ?? []).map((week) =>
      week.zoneSeconds.reduce((sum, seconds) => sum + seconds, 0),
    ),
    1,
  );

  return (
    <PageShell>
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-5">
        <h1 className="font-semibold text-2xl tracking-tight">Fitness</h1>
        {isPending ? (
          <Skeleton className="h-64 w-full" role="status" aria-label="Loading the timeline" />
        ) : isError ? (
          <p className="text-sm text-[var(--alert)]" role="alert">
            The service did not say what the training has come to.
          </p>
        ) : data.days.length === 0 ? (
          <p className="text-[var(--ink-2)] text-sm">
            Nothing has been worked out yet. Training load needs the numbers on{" "}
            <Link className="underline" to="/settings">
              settings
            </Link>
            , and a ride recorded with a heart-rate strap or a power meter.
          </p>
        ) : (
          <>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <ToggleGroup
                aria-label="Scale"
                variant="outline"
                spacing={0}
                value={[scale]}
                onValueChange={(next) => {
                  // Pressing the pressed one empties the group; the chart is
                  // always on one scale, so that leaves it as it was.
                  const chosen = next[0];
                  if (chosen === "tss" || chosen === "trimp") {
                    setScale(chosen);
                  }
                }}
              >
                {SCALES.map(({ value, label }) => (
                  <ToggleGroupItem key={value} value={value}>
                    {label}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
              <ToggleGroup
                aria-label="Range"
                variant="outline"
                spacing={0}
                value={[range]}
                onValueChange={(next) => {
                  const chosen = next[0];
                  if (chosen) {
                    setRange(chosen);
                  }
                }}
              >
                {RANGES.map(({ value, label }) => (
                  <ToggleGroupItem key={value} value={value}>
                    {label}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
            </div>
            <div className="rounded-xl bg-[var(--panel)] p-3 ring-1 ring-black/5">
              <FitnessChart days={data.days} scale={scale} />
              <ul className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-[var(--ink-2)] text-xs">
                <li>Fitness — six weeks of work</li>
                <li>Fatigue — one week of it</li>
                <li>Form — the difference, fresh above zero</li>
              </ul>
            </div>
            {activities.isError ? (
              // An outage must not read as a season with nothing in it: the
              // panel says the rides were not read rather than disappearing.
              <p className="text-sm text-[var(--alert)]" role="alert">
                The service did not say what has been ridden, so the season's decoupling is not
                drawn.
              </p>
            ) : decoupling.length > 0 ? (
              <div className="rounded-xl bg-[var(--panel)] p-3 ring-1 ring-black/5">
                <h2 className="font-semibold text-lg">Decoupling</h2>
                <DecouplingChart points={decoupling} />
                <p className="mt-2 text-[var(--ink-2)] text-xs">
                  Per ride, the share of the first half's power-to-heart-rate ratio lost over the
                  second. A falling cloud is aerobic fitness arriving. Measured power only, over
                  rides of an hour or more, and only meaningful for a steady one.
                </p>
              </div>
            ) : null}
            {data.powerCurve && data.powerCurve.length > 0 ? (
              <div className="rounded-xl bg-[var(--panel)] p-3 ring-1 ring-black/5">
                <h2 className="font-semibold text-lg">Power duration</h2>
                <PowerCurveChart points={data.powerCurve} />
                <ul className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-[var(--ink-2)] text-xs">
                  {data.powerCurve.map((point) => (
                    <li key={point.seconds}>
                      {formatCurveDuration(point.seconds)} — {Math.round(point.watts)} W
                    </li>
                  ))}
                </ul>
                <p className="mt-2 text-[var(--ink-2)] text-xs">
                  The best each duration reached over this window, from measured power alone. A
                  duration no ride was long enough for carries no point.
                </p>
              </div>
            ) : null}
            {data.weeks.length > 0 ? (
              <>
                <h2 className="font-semibold text-lg">Time in zone, by week</h2>
                <ul className="flex flex-col gap-2">
                  {data.weeks.map((week) => (
                    <WeekBar key={week.weekStart} week={week} widest={widest} />
                  ))}
                </ul>
              </>
            ) : null}
          </>
        )}
      </div>
    </PageShell>
  );
}
