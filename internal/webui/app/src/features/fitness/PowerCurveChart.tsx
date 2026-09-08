/**
 * The power-duration curve: the best mean power held at each duration over the
 * window, shortest on the left.
 *
 * Drawn on a logarithmic time axis, because the durations span five seconds to
 * an hour and a linear axis would crush every short effort against the origin.
 * The shape a rider reads — a steep sprinter's fall, a flat diesel's plateau —
 * only exists on the log axis.
 */

import { useId } from "react";
import type { PowerCurvePoint } from "../../api/types";

const HEIGHT = 180;
const WIDTH = 720;
const PADDING = { top: 12, right: 8, bottom: 24, left: 40 };

/** How a duration is said in the axis and the label: seconds, then minutes. */
export function formatCurveDuration(seconds: number): string {
  return seconds < 60 ? `${seconds} s` : `${Math.round(seconds / 60)} min`;
}

export function PowerCurveChart({ points }: { points: PowerCurvePoint[] }) {
  const titleId = useId();
  if (points.length === 0) {
    return null;
  }
  const plotWidth = WIDTH - PADDING.left - PADDING.right;
  const plotHeight = HEIGHT - PADDING.top - PADDING.bottom;
  const highest = Math.max(...points.map((one) => one.watts));
  const x = (seconds: number) => {
    const first = Math.log(points[0]?.seconds ?? 1);
    const last = Math.log(points[points.length - 1]?.seconds ?? 1);
    const span = last - first || 1;

    return PADDING.left + ((Math.log(seconds) - first) / span) * plotWidth;
  };
  // Watts run from nought, so the fall across the curve is read against the
  // whole figure rather than against whatever the shortest effort reached.
  const y = (watts: number) => PADDING.top + plotHeight - (watts / (highest || 1)) * plotHeight;

  return (
    <svg
      viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
      className="w-full"
      role="img"
      aria-labelledby={titleId}
      preserveAspectRatio="none"
    >
      <title id={titleId}>
        Best mean power over {points.map((one) => formatCurveDuration(one.seconds)).join(", ")}
      </title>
      <polyline
        points={points.map((one) => `${x(one.seconds)},${y(one.watts)}`).join(" ")}
        fill="none"
        stroke="var(--accent)"
        strokeWidth={1.5}
      />
      {points.map((one) => (
        <circle
          key={one.seconds}
          cx={x(one.seconds)}
          cy={y(one.watts)}
          r={3}
          fill="var(--accent)"
        />
      ))}
    </svg>
  );
}
