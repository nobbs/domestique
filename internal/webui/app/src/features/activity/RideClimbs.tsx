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

import { IconStairs } from "@tabler/icons-react";
import type { RouteClimb, RouteClimbAttempt } from "../../api/types";
import { PanelHeading } from "../../components/PanelHeading";
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

const HEAD = "pb-2 text-left font-normal text-[var(--ink-2)] text-xs";
const CELL = "border-[var(--rule)] border-t py-3 align-top text-sm tabular-nums";

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
  // Green for the quickest so far, amber for any other placing; the tone is a
  // claim about this ride, and a first attempt makes none.
  const standingTone = rank === 0 ? "var(--good)" : "var(--hold)";
  const beaten = hasEarlierAttempts && best && best.activityId !== mine.activityId;

  return (
    <tr>
      <td className={CELL}>
        <div className="font-medium">Climb {ordinal}</div>
        <div className="text-[var(--ink-2)] text-xs">
          {formatDistance(climb.distanceMetres)} · {formatGradient(climb.averageGradePercent)} avg ·{" "}
          {formatGradient(climb.maxGradePercent)} max
        </div>
      </td>
      <td className={CELL}>{formatClimbTime(mine.seconds)}</td>
      <td className={`${CELL} text-[var(--ink-2)]`}>
        {mine.heartRateBpm !== undefined ? `${Math.round(mine.heartRateBpm)} bpm` : "—"}
      </td>
      <td className={`${CELL} text-[var(--ink-2)]`}>{attemptPower(mine)}</td>
      <td className={`${CELL} text-right font-medium`}>
        {hasEarlierAttempts ? (
          <span style={{ color: standingTone }}>
            #{rank + 1} of {total}
          </span>
        ) : (
          <span className="font-normal text-[var(--ink-2)]">first attempt</span>
        )}
      </td>
      <td className={`${CELL} text-right`}>
        {beaten ? formatClimbTime(best.seconds) : <span className="text-[var(--ink-2)]">—</span>}
      </td>
    </tr>
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
      className="flex flex-col gap-3 rounded-xl bg-[var(--panel)] p-4 shadow-[var(--shadow)]"
      aria-label="By the climb"
    >
      <PanelHeading icon={<IconStairs size={18} stroke={1.8} />} title="By the climb" />
      <table className="w-full border-collapse">
        <thead>
          <tr>
            <th className={HEAD}>Climb</th>
            <th className={HEAD}>Time</th>
            <th className={HEAD}>Heart rate</th>
            <th className={HEAD}>Power</th>
            <th className={`${HEAD} text-right`}>Standing</th>
            <th className={`${HEAD} text-right`}>Best before</th>
          </tr>
        </thead>
        <tbody>
          {ridden.map(({ climb, ordinal }) => (
            <ClimbRow
              key={climb.startMetres}
              climb={climb}
              ordinal={ordinal}
              activityId={activityId}
            />
          ))}
        </tbody>
      </table>
    </section>
  );
}
