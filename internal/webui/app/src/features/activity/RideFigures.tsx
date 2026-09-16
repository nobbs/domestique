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

import {
  IconArrowBarDown,
  IconArrowBarUp,
  IconBolt,
  IconFlame,
  IconRuler2,
  IconStopwatch,
} from "@tabler/icons-react";
import type { ComponentType } from "react";
import type { Activity } from "../../api/types";
import { Badge } from "../../components/ui/badge";
import {
  formatAscent,
  formatCoverage,
  formatDescent,
  formatDistance,
  formatDuration,
} from "../../lib/format";

/** A figure's own note, with a coverage share appended where the series held less than the whole ride. */
function withCoverage(note: string | undefined, coverage: number | undefined): string | undefined {
  const coverageNote = formatCoverage(coverage);
  if (!coverageNote) {
    return note;
  }

  return note ? `${note} · ${coverageNote}` : coverageNote;
}

interface Headline {
  label: string;
  value: string;
  unit?: string;
  note?: string;
}

/** The mark each figure wears in its cell; keyed by label so the figures stay plain data. */
const MARKS: Record<
  string,
  ComponentType<{ size?: number; stroke?: number; "aria-hidden"?: "true" }>
> = {
  Distance: IconRuler2,
  Moving: IconStopwatch,
  Climbed: IconArrowBarUp,
  Descended: IconArrowBarDown,
  Calories: IconFlame,
  "Calories (est.)": IconFlame,
  "Training stress": IconBolt,
};

/** Which load scale the ride can be named on, and what it came to. */
function loadFigure(ride: Activity): Headline | null {
  const metrics = ride.metrics;
  if (metrics?.powerTss !== undefined) {
    const note = withCoverage(
      metrics.intensityFactor !== undefined
        ? `${metrics.intensityFactor.toFixed(2)} of threshold`
        : undefined,
      metrics.powerCoverage,
    );

    return {
      label: "Training stress",
      value: metrics.powerTss.toFixed(0),
      unit: "TSS",
      ...(note !== undefined ? { note } : {}),
    };
  }
  if (metrics?.heartRateTss !== undefined) {
    const note = withCoverage(undefined, metrics.heartRateCoverage);

    return {
      label: "Training stress",
      value: metrics.heartRateTss.toFixed(0),
      unit: "hrTSS",
      ...(note !== undefined ? { note } : {}),
    };
  }
  if (metrics?.trimp !== undefined) {
    const note = withCoverage(undefined, metrics.heartRateCoverage);

    return {
      label: "Training impulse",
      value: metrics.trimp.toFixed(0),
      unit: "TRIMP",
      ...(note !== undefined ? { note } : {}),
    };
  }

  return null;
}

/**
 * The device's own reported calories, with the power-based estimate as a note
 * beside it for comparison; the estimate stands alone where the device
 * declared none, and neither yields no headline at all.
 */
function caloriesFigure(ride: Activity): Headline[] {
  const estimated = ride.metrics?.estimatedCaloriesKcal;
  if (ride.caloriesKcal !== undefined) {
    return [
      {
        label: "Calories",
        value: ride.caloriesKcal.toFixed(0),
        unit: "kcal",
        ...(estimated !== undefined ? { note: `~${estimated.toFixed(0)} kcal estimated` } : {}),
      },
    ];
  }
  if (estimated !== undefined) {
    return [{ label: "Calories (est.)", value: estimated.toFixed(0), unit: "kcal" }];
  }

  return [];
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
    ...caloriesFigure(ride),
    ...(load ? [load] : []),
  ];

  return (
    <dl
      className="grid grid-cols-2 overflow-hidden rounded-2xl bg-[var(--panel)] shadow-[var(--shadow)]"
      aria-label="Ride figures"
    >
      {ride.provider === "zwift" ? (
        <div className="col-span-2 flex flex-col gap-1 border-[var(--rule)] border-b px-4 py-3">
          <dt className="sr-only">Recorded on</dt>
          <dd>
            <Badge variant="secondary" className="w-fit">
              Zwift
            </Badge>
          </dd>
          {ride.workoutName !== undefined ? (
            // The name is what Zwift lists the ride under, a route for a free
            // ride; only a workout can fall short, so completion shows when it did.
            <dd className="text-[var(--ink-2)] text-sm">
              {ride.workoutName}
              {ride.workoutCompletion !== undefined && ride.workoutCompletion < 1
                ? `, ${(ride.workoutCompletion * 100).toFixed(0)} % completed`
                : ""}
            </dd>
          ) : null}
        </div>
      ) : null}
      {figures.map((figure, index) => {
        const Mark = MARKS[figure.label];
        return (
          // Hairlines between cells rather than around them: a rule on the
          // right of every left cell, and above every row but the first.
          <div
            key={figure.label}
            className={`flex gap-3 border-[var(--rule)] px-4 py-3 ${index >= 2 ? "border-t" : ""} ${index % 2 === 1 ? "" : index === figures.length - 1 ? "col-span-2" : "border-r"}`}
          >
            {Mark ? (
              <span className="grid size-8 shrink-0 place-items-center rounded-md bg-[var(--ink)] text-[var(--panel)]">
                <Mark size={16} stroke={1.8} aria-hidden="true" />
              </span>
            ) : null}
            <div className="flex min-w-0 flex-col gap-0.5">
              <dt className="text-[11px] text-[var(--ink-2)]">{figure.label}</dt>
              <dd className="font-semibold text-2xl leading-tight tabular-nums tracking-tight">
                {figure.value}
                {figure.unit ? (
                  <span className="ml-1 font-normal text-[var(--ink-2)] text-sm">
                    {figure.unit}
                  </span>
                ) : null}
              </dd>
              {figure.note ? <dd className="text-[var(--ink-2)] text-xs">{figure.note}</dd> : null}
            </div>
          </div>
        );
      })}
    </dl>
  );
}
