/**
 * The figures that decide a ride, set large beside the map: how far, how
 * long, how much climbing, and how hard, with the descent and calories where
 * the file declared them. Everything else the ride's sensors averaged lives in
 * the effort panel below.
 *
 * The load figure is whichever scale the ride allowed, most specific first: a
 * power TSS where the bicycle carried a meter, a heart-rate TSS where it
 * carried a strap, Banister's TRIMP otherwise — and none for a ride whose
 * recorded file was never readable.
 */

import type { Activity } from "../../api/types";
import { Badge } from "../../components/ui/badge";
import { formatAscent, formatDescent, formatDistance, formatDuration } from "../../lib/format";

interface Headline {
  label: string;
  value: string;
  unit?: string;
  note?: string;
}

/** Which load scale the ride can be named on, and what it came to. */
function loadFigure(ride: Activity): Headline | null {
  const metrics = ride.metrics;
  if (metrics?.powerTss !== undefined) {
    return {
      label: "Training stress",
      value: metrics.powerTss.toFixed(0),
      unit: "TSS",
      ...(metrics.intensityFactor !== undefined
        ? { note: `${metrics.intensityFactor.toFixed(2)} of threshold` }
        : {}),
    };
  }
  if (metrics?.heartRateTss !== undefined) {
    return { label: "Training stress", value: metrics.heartRateTss.toFixed(0), unit: "hrTSS" };
  }
  if (metrics?.trimp !== undefined) {
    return { label: "Training impulse", value: metrics.trimp.toFixed(0), unit: "TRIMP" };
  }

  return null;
}

export function RideFigures({ ride }: { ride: Activity | undefined }) {
  if (!ride) {
    return null;
  }
  const load = loadFigure(ride);
  const figures: Headline[] = [
    { label: "Distance", value: formatDistance(ride.distanceMetres) },
    {
      label: "Moving",
      value: formatDuration(ride.movingSeconds),
      // Elapsed time is only worth a reader's eye where it says something
      // moving time did not.
      ...(ride.elapsedSeconds - ride.movingSeconds >= 60
        ? { note: `${formatDuration(ride.elapsedSeconds)} elapsed` }
        : {}),
    },
    { label: "Climbed", value: formatAscent(ride.ascentMetres) },
    ...(ride.descentMetres !== undefined
      ? [{ label: "Descended", value: formatDescent(ride.descentMetres) }]
      : []),
    ...(ride.caloriesKcal !== undefined
      ? [{ label: "Calories", value: ride.caloriesKcal.toFixed(0), unit: "kcal" }]
      : []),
    ...(load ? [load] : []),
  ];

  return (
    <dl className="grid grid-cols-2 gap-x-6 gap-y-5" aria-label="Ride figures">
      {ride.provider === "zwift" ? (
        <div className="col-span-2 flex flex-col gap-1">
          <dt className="sr-only">Recorded on</dt>
          <dd>
            <Badge variant="secondary" className="w-fit">
              Zwift
            </Badge>
          </dd>
          {ride.workoutName !== undefined ? (
            <dd className="text-[var(--ink-2)] text-sm">
              Workout: {ride.workoutName}
              {ride.workoutCompletion !== undefined
                ? `, ${(ride.workoutCompletion * 100).toFixed(0)} % completed`
                : ""}
            </dd>
          ) : null}
        </div>
      ) : null}
      {figures.map((figure) => (
        <div key={figure.label} className="flex flex-col gap-0.5">
          <dt className="font-semibold text-[10px] text-[var(--ink-2)] uppercase tracking-[0.08em]">
            {figure.label}
          </dt>
          <dd className="font-semibold text-4xl tabular-nums tracking-tight">
            {figure.value}
            {figure.unit ? (
              <span className="ml-1 font-normal text-[var(--ink-2)] text-sm">{figure.unit}</span>
            ) : null}
          </dd>
          {figure.note ? <dd className="text-[var(--ink-2)] text-xs">{figure.note}</dd> : null}
        </div>
      ))}
    </dl>
  );
}
