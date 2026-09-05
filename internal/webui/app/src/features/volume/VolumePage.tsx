/**
 * Volume: what the rider has actually ridden, a year back.
 *
 * Every other page is about routes the service holds for them; this is the one
 * about rides they have already done, read from the activity summaries their
 * target recorded. Weeks and months are the two periods training is counted in,
 * so they are the only two offered.
 */

import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { Link } from "react-router";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { activitiesQuery } from "../../api/queries";
import { PageShell } from "../../components/Layout";
import { Skeleton } from "../../components/ui/skeleton";
import { formatAscent, formatCount, formatDistance, formatMovingTime } from "../../lib/format";
import {
  bucketActivities,
  type Granularity,
  type VolumeTotals,
  volumeTotals,
  windowStart,
} from "../../lib/volume";

const GRANULARITIES: ReadonlyArray<{ value: Granularity; label: string; heading: string }> = [
  { value: "week", label: "Week", heading: "By week" },
  { value: "month", label: "Month", heading: "By month" },
];

export function VolumePage() {
  const [granularity, setGranularity] = useState<Granularity>("week");
  const from = useMemo(() => windowStart(), []);
  const { data, isPending, isError } = useQuery(activitiesQuery(from));
  const activities = useMemo(() => data ?? [], [data]);
  const buckets = useMemo(
    () => bucketActivities(activities, granularity),
    [activities, granularity],
  );
  const totals = useMemo(() => volumeTotals(activities), [activities]);
  const widest = Math.max(...buckets.map((bucket) => bucket.distanceMetres), 1);
  const heading = GRANULARITIES.find(({ value }) => value === granularity)?.heading;

  return (
    <PageShell>
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-5">
        <h1 className="font-semibold text-2xl tracking-tight">Volume</h1>
        {isPending ? (
          <Skeleton className="h-64 w-full" role="status" aria-label="Loading activities" />
        ) : isError ? (
          <p className="text-sm text-[var(--alert)]">
            The service did not say what has been ridden.
          </p>
        ) : totals.count === 0 ? (
          <p className="text-[var(--ink-2)] text-sm">
            No rides have been recorded yet. Once a Wahoo account is connected on{" "}
            <Link className="underline" to="/settings">
              settings
            </Link>
            , the rides it records appear here.
          </p>
        ) : (
          <>
            <Totals totals={totals} />
            <div className="flex items-center justify-between gap-3">
              <h2 className="font-semibold text-lg">{heading}</h2>
              <ToggleGroup
                aria-label="Period"
                variant="outline"
                spacing={0}
                value={[granularity]}
                onValueChange={(next) => {
                  // Pressing the pressed one empties the group; the page is
                  // always by week or by month, so that leaves it as it was.
                  const chosen = next[0];
                  if (chosen === "week" || chosen === "month") {
                    setGranularity(chosen);
                  }
                }}
              >
                {GRANULARITIES.map(({ value, label }) => (
                  <ToggleGroupItem key={value} value={value}>
                    {label}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
            </div>
            <ul className="flex flex-col gap-2">
              {buckets.map((bucket) => (
                <li
                  key={bucket.start.toISOString()}
                  className="grid grid-cols-[5.5rem_1fr] items-center gap-3"
                >
                  <span className="text-[var(--ink-2)] text-sm">{bucket.label}</span>
                  <div className="flex flex-col gap-1">
                    <div
                      aria-hidden="true"
                      className="h-2 min-w-px rounded-full bg-[var(--accent)]"
                      style={{ width: `${(bucket.distanceMetres / widest) * 100}%` }}
                    />
                    <span className="text-[var(--ink-2)] text-xs">
                      {bucket.count === 0
                        ? "No rides"
                        : [
                            formatDistance(bucket.distanceMetres),
                            formatMovingTime(bucket.movingSeconds),
                            formatAscent(bucket.ascentMetres),
                            formatCount(bucket.count, "ride"),
                          ].join(" · ")}
                    </span>
                  </div>
                </li>
              ))}
            </ul>
          </>
        )}
      </div>
    </PageShell>
  );
}

function Totals({ totals }: { totals: VolumeTotals }) {
  const figures = [
    { label: "Distance", value: formatDistance(totals.distanceMetres) },
    { label: "Moving time", value: formatMovingTime(totals.movingSeconds) },
    { label: "Ascent", value: formatAscent(totals.ascentMetres) },
    { label: "Rides", value: totals.count.toLocaleString() },
  ];

  return (
    <dl className="grid grid-cols-2 gap-3 rounded-xl bg-[var(--panel)] p-4 ring-1 ring-black/5 sm:grid-cols-4">
      {figures.map(({ label, value }) => (
        <div key={label} className="flex flex-col gap-0.5">
          <dt className="text-[var(--ink-2)] text-xs">{label}</dt>
          <dd className="font-semibold text-lg">{value}</dd>
        </div>
      ))}
    </dl>
  );
}
