/**
 * Aerobic decoupling, one dot per ride, over the range the page is showing.
 *
 * Dots rather than a line: decoupling is a property of each ride, not a
 * quantity that carries from one to the next, and joining them would draw a
 * slope through days nobody rode. What a reader is after is the drift of the
 * cloud — a falling one is the plain signal of aerobic fitness.
 */

import { useId } from "react";
import type { Activity } from "../../api/types";

const HEIGHT = 140;
const WIDTH = 720;
const PADDING = { top: 12, right: 8, bottom: 20, left: 36 };

/** One ride that carries a decoupling figure, at the moment it was ridden. */
export interface DecouplingPoint {
  /** The ride's own id: two rides can start at the same moment on two targets. */
  id: string;
  at: number;
  percent: number;
}

/**
 * The rides in the window that carry a decoupling figure, oldest first. A ride
 * without one is not a zero: it had no meter, or was too short to have drifted.
 */
export function decouplingPoints(activities: Activity[], from: Date): DecouplingPoint[] {
  const after = from.getTime();

  return activities
    .flatMap((ride) => {
      const percent = ride.metrics?.decouplingPercent;
      const at = Date.parse(ride.startedAt);
      if (percent === undefined || !Number.isFinite(at) || at < after) {
        return [];
      }

      return [{ id: ride.id, at, percent }];
    })
    .sort((one, other) => one.at - other.at);
}

export function DecouplingChart({ points }: { points: DecouplingPoint[] }) {
  const titleId = useId();
  if (points.length === 0) {
    return null;
  }
  // Zero is in the frame whichever way the rides went: it is the line a reading
  // is read against, and a cloud drawn without it says nothing.
  const highest = Math.max(...points.map((one) => one.percent), 0);
  const lowest = Math.min(...points.map((one) => one.percent), 0);
  const span = highest - lowest || 1;
  const earliest = points[0]?.at ?? 0;
  const latest = points[points.length - 1]?.at ?? earliest;
  const elapsed = latest - earliest || 1;

  const plotWidth = WIDTH - PADDING.left - PADDING.right;
  const plotHeight = HEIGHT - PADDING.top - PADDING.bottom;
  const x = (at: number) => PADDING.left + ((at - earliest) / elapsed) * plotWidth;
  const y = (percent: number) =>
    PADDING.top + plotHeight - ((percent - lowest) / span) * plotHeight;

  return (
    <svg
      viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
      className="w-full"
      role="img"
      aria-labelledby={titleId}
      preserveAspectRatio="none"
    >
      <title id={titleId}>
        Aerobic decoupling over {points.length} {points.length === 1 ? "ride" : "rides"}, as a
        percentage of the first half's power-to-heart-rate ratio
      </title>
      <line
        x1={PADDING.left}
        x2={WIDTH - PADDING.right}
        y1={y(0)}
        y2={y(0)}
        stroke="var(--rule)"
        strokeWidth={1}
      />
      {points.map((one) => (
        <circle
          key={one.id}
          cx={x(one.at)}
          cy={y(one.percent)}
          r={3}
          fill="var(--accent)"
          fillOpacity={0.75}
        />
      ))}
    </svg>
  );
}
