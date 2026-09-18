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
import {
  formatAscent,
  formatCoverage,
  formatDescent,
  formatDistance,
  formatDuration,
} from "../../lib/format";
import { WHOLE_LAP_COVERAGE } from "../../lib/rideHistory";
import { climbingVerdict, intensityVerdict, type VerdictTone } from "../../lib/verdict";

/** A figure's own note, with a coverage share appended where the series held less than the whole ride. */
function withCoverage(note: string | undefined, coverage: number | undefined): string | undefined {
  const coverageNote = formatCoverage(coverage);
  if (!coverageNote) {
    return note;
  }

  return note ? `${note} · ${coverageNote}` : coverageNote;
}

/** The page's tones plus a quiet one for figures that make no claim. */
export type Tone = VerdictTone | "quiet";

export interface Headline {
  label: string;
  value: string;
  unit?: string;
  /** What the figure adds up to, in a tone; leads the note where there is one. */
  verdict?: { label: string; tone: Tone };
  note?: string;
  tone: Tone;
}

export function colour(tone: Tone): string {
  return tone === "quiet" ? "var(--ink)" : tone === "info" ? "var(--accent)" : `var(--${tone})`;
}

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

    const verdict = intensityVerdict(metrics.intensityFactor);

    return {
      label: "Training stress",
      value: metrics.powerTss.toFixed(0),
      unit: "TSS",
      tone: verdict?.tone ?? "alert",
      ...(verdict ? { verdict } : {}),
      ...(note !== undefined ? { note } : {}),
    };
  }
  if (metrics?.heartRateTss !== undefined) {
    const note = withCoverage(undefined, metrics.heartRateCoverage);

    return {
      label: "Training stress",
      value: metrics.heartRateTss.toFixed(0),
      unit: "hrTSS",
      tone: "alert",
      ...(note !== undefined ? { note } : {}),
    };
  }
  if (metrics?.trimp !== undefined) {
    const note = withCoverage(undefined, metrics.heartRateCoverage);

    return {
      label: "Training impulse",
      value: metrics.trimp.toFixed(0),
      unit: "TRIMP",
      tone: "alert",
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
        tone: "quiet",
        value: ride.caloriesKcal.toFixed(0),
        unit: "kcal",
        ...(estimated !== undefined ? { note: `~${estimated.toFixed(0)} kcal estimated` } : {}),
      },
    ];
  }
  if (estimated !== undefined) {
    return [{ label: "Calories (est.)", value: estimated.toFixed(0), unit: "kcal", tone: "quiet" }];
  }

  return [];
}

/** The ride's headline figures, in the order the hero reads them. */
export function rideHeadlines(ride: Activity): Headline[] {
  const load = loadFigure(ride);
  const stoppedSeconds = ride.elapsedSeconds - ride.movingSeconds;
  // A dash for the ascent is "no usable profile", not a flat ride; nothing to grade.
  const climbing =
    ride.ascentMetres > 0 ? climbingVerdict(ride.ascentMetres, ride.distanceMetres) : null;
  const match = ride.routeMatch;
  return [
    {
      label: "Distance",
      value: formatDistance(ride.distanceMetres),
      tone: "info",
      ...(match
        ? {
            verdict: {
              label:
                match.routeCoverage >= WHOLE_LAP_COVERAGE
                  ? "Whole route"
                  : `${Math.round(match.routeCoverage * 100)}% of the route`,
              tone: "info",
            },
          }
        : {}),
    },
    {
      label: "Moving",
      value: formatDuration(ride.movingSeconds),
      tone: "quiet",
      // Elapsed time is only worth a reader's eye where it says something
      // moving time did not.
      ...(stoppedSeconds >= 60
        ? {
            verdict: { label: `${Math.round(stoppedSeconds / 60)} min stopped`, tone: "quiet" },
            note: `${formatDuration(ride.elapsedSeconds)} elapsed`,
          }
        : {}),
    },
    {
      label: "Climbed",
      value: formatAscent(ride.ascentMetres),
      tone: climbing?.tone ?? "hold",
      ...(climbing ? { verdict: climbing } : {}),
      ...(climbing
        ? { note: `${Math.round(ride.ascentMetres / (ride.distanceMetres / 1000))} m per km` }
        : {}),
    },
    ...(ride.descentMetres !== undefined
      ? [{ label: "Descended", value: formatDescent(ride.descentMetres), tone: "quiet" as const }]
      : []),
    ...caloriesFigure(ride),
    ...(load ? [load] : []),
  ];
}

export function RideFigures({ ride }: { ride: Activity | undefined }) {
  if (!ride) {
    return null;
  }
  const figures = rideHeadlines(ride);

  return (
    <dl
      className="grid grid-cols-2 gap-x-5 gap-y-4 sm:grid-cols-3 lg:grid-cols-6"
      aria-label="Ride figures"
    >
      {ride.provider === "zwift" ? (
        <div className="col-span-full flex flex-col gap-1">
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
      {figures.map((figure) => (
        <div key={figure.label} className="flex min-w-0 flex-col gap-1.5">
          <dt className="text-[var(--ink-2)] text-sm">{figure.label}</dt>
          <dd
            className="rounded-lg px-3 py-2 font-semibold text-xl leading-tight tabular-nums"
            style={{
              color: colour(figure.tone),
              background: `color-mix(in oklab, ${figure.tone === "quiet" ? "var(--ink-2)" : colour(figure.tone)} 14%, transparent)`,
            }}
          >
            {figure.value}
            {figure.unit ? <span className="ml-1.5 font-normal text-sm">{figure.unit}</span> : null}
          </dd>
          {figure.verdict || figure.note ? (
            <dd className="text-sm">
              {figure.verdict ? (
                <span className="font-medium" style={{ color: colour(figure.verdict.tone) }}>
                  {figure.verdict.label}
                </span>
              ) : null}
              {figure.verdict && figure.note ? (
                <span className="text-[var(--ink-2)]"> · </span>
              ) : null}
              {figure.note ? <span className="text-[var(--ink-2)]">{figure.note}</span> : null}
            </dd>
          ) : null}
        </div>
      ))}
    </dl>
  );
}
