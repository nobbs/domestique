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

/**
 * This attempt's standing among the rider's attempts up to and including it
 * — never a later ride's, which `attempts`' quickest-first order alone
 * cannot tell apart from an earlier one. A ride from before any other
 * attempt on this climb existed has nothing to rank against or beat, however
 * fast a later ride went.
 */
function standingAt(attempts: RouteClimbAttempt[], mine: RouteClimbAttempt) {
  const myTime = Date.parse(mine.riddenAt);
  const soFar = attempts.filter((attempt) => Date.parse(attempt.riddenAt) <= myTime);
  const bySpeed = [...soFar].sort((a, b) => a.seconds - b.seconds);
  const rank = bySpeed.findIndex((attempt) => attempt.activityId === mine.activityId);
  const best = bySpeed[0];

  return { hasEarlierAttempts: soFar.length > 1, rank, total: soFar.length, best };
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
  const mine = climb.attempts.find((attempt) => attempt.activityId === activityId);
  if (!mine) {
    return null;
  }
  const { hasEarlierAttempts, rank, total, best } = standingAt(climb.attempts, mine);

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
          {hasEarlierAttempts ? ` · #${rank + 1} of ${total}` : ""}
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
  // Ordinal from the climb's position in the route's own order — the same
  // one ClimbsSidebar's list and its chart bracket carry — not from position
  // after filtering, which would renumber a climb a partial lap skipped.
  const ridden = (climbs ?? [])
    .map((climb, index) => ({ climb, ordinal: index + 1 }))
    .filter(({ climb }) => climb.attempts.some((attempt) => attempt.activityId === activityId));
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
        {ridden.map(({ climb, ordinal }) => (
          <ClimbRow
            key={climb.startMetres}
            climb={climb}
            ordinal={ordinal}
            activityId={activityId}
          />
        ))}
      </div>
    </section>
  );
}
