/**
 * A matched ride's attempts at its route's sustained climbs: one row per
 * climb this ride rode, the rider's rank there and the best time among
 * earlier attempts, where more than this one exists.
 *
 * Absent for a ride with no route match, a route with no sustained climb, or
 * a ride that rode none of them — the profile, the series chart and the
 * effort panel already show the ride's shape, which was the #646 spike's own
 * test for dropping "By the kilometre" in this panel's place.
 */

import type { RouteClimb, RouteClimbAttempt } from "../../api/types";
import { formatClimbTime, formatDistance, formatGradient } from "../../lib/format";

/** Measured power only if the bicycle carried a meter; an estimate never sums or ranks against it. */
function attemptPower(attempt: RouteClimbAttempt): string {
  if (attempt.powerWatts !== undefined) {
    return `${Math.round(attempt.powerWatts)} W`;
  }
  if (attempt.estimatedPowerWatts !== undefined) {
    return `~${Math.round(attempt.estimatedPowerWatts)} W`;
  }

  return "—";
}

function ClimbRow({
  climb,
  ordinal,
  activityId,
}: {
  climb: RouteClimb;
  ordinal: number;
  activityId: string;
}) {
  const rank = climb.attempts.findIndex((attempt) => attempt.activityId === activityId);
  const mine = climb.attempts[rank];
  if (!mine) {
    return null;
  }
  const best = climb.attempts[0];
  // A rider's first-ever attempt has nothing to rank against or beat.
  const hasEarlierAttempts = climb.attempts.length > 1;

  return (
    <div className="flex items-start justify-between gap-4 border-[var(--rule)] border-t pt-2 text-sm first:border-t-0 first:pt-0">
      <div>
        <div className="font-medium">Climb {ordinal}</div>
        <div className="text-[var(--ink-2)] text-xs tabular-nums">
          {formatDistance(climb.distanceMetres)} · {formatGradient(climb.averageGradePercent)} avg ·{" "}
          {formatGradient(climb.maxGradePercent)} max
        </div>
      </div>
      <div className="text-right text-xs tabular-nums">
        <div className="text-[var(--ink)]">
          {formatClimbTime(mine.seconds)}
          {hasEarlierAttempts ? ` · #${rank + 1} of ${climb.attempts.length}` : ""}
        </div>
        <div className="text-[var(--ink-2)]">
          {mine.heartRateBpm !== undefined ? `${Math.round(mine.heartRateBpm)} bpm · ` : ""}
          {attemptPower(mine)}
        </div>
        {hasEarlierAttempts && best && best.activityId !== mine.activityId ? (
          <div className="text-[var(--ink-2)]">best here {formatClimbTime(best.seconds)}</div>
        ) : null}
      </div>
    </div>
  );
}

export interface RideClimbsProps {
  climbs: RouteClimb[] | undefined;
  activityId: string;
}

export function RideClimbs({ climbs, activityId }: RideClimbsProps) {
  const ridden = (climbs ?? []).filter((climb) =>
    climb.attempts.some((attempt) => attempt.activityId === activityId),
  );
  if (ridden.length === 0) {
    return null;
  }

  return (
    <section
      className="flex flex-col gap-3 rounded-xl bg-[var(--panel)] p-4 ring-1 ring-black/5"
      aria-label="By the climb"
    >
      <h2 className="font-medium text-sm">By the climb</h2>
      <div className="flex flex-col gap-2">
        {ridden.map((climb, index) => (
          <ClimbRow
            key={climb.startMetres}
            climb={climb}
            ordinal={index + 1}
            activityId={activityId}
          />
        ))}
      </div>
    </section>
  );
}
