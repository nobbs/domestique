/**
 * Ground to stand a panel on.
 *
 * A card judged on a white page has already won the argument the pill exists
 * to settle — how much of the map it gives back — without being asked it. So
 * every story here puts the panel where the shell puts it, over something.
 *
 * The something is synthetic: contour rings and the fixture's own loop
 * projected into the frame, rather than a fetched basemap. That is enough to
 * judge coverage and the panel's contrast against a busy background, and it is
 * not enough to judge legibility over satellite imagery — which is a question
 * for a real map and a real screenshot.
 */

import type { ReactNode } from "react";
import type { Position } from "../../../api/types";

export const PANE = { width: 860, height: 540 };

/** Rings standing in for contour lines, so the ground has some texture on it. */
const CONTOURS = Array.from({ length: 13 }, (_, index) => 90 + index * 34);

/** The route projected into the frame, so the map has this ride on it. */
function routePath(coordinates: Position[]): string {
  const longitudes = coordinates.map((point) => point[0]);
  const latitudes = coordinates.map((point) => point[1]);
  const west = Math.min(...longitudes);
  const east = Math.max(...longitudes);
  const south = Math.min(...latitudes);
  const north = Math.max(...latitudes);
  const scale = Math.min((PANE.width - 360) / (east - west), (PANE.height - 140) / (north - south));

  // Every eighth point: the fixtures carry a couple of thousand, and a path
  // with that many commands in it is a slow way to draw the same line.
  return coordinates
    .filter((_, index) => index % 8 === 0)
    .map(([longitude, latitude], index) => {
      const x = PANE.width - 60 - (east - longitude) * scale;
      const y = 70 + (north - latitude) * scale;

      return `${index === 0 ? "M" : "L"}${x.toFixed(1)} ${y.toFixed(1)}`;
    })
    .join(" ");
}

export function MapPane({
  coordinates,
  children,
}: {
  coordinates: Position[];
  /** The panel, placed where the shell places it. */
  children: ReactNode;
}) {
  return (
    <div
      className="relative overflow-hidden rounded-lg ring-1 ring-[var(--rule)]"
      style={{ width: PANE.width, height: PANE.height }}
    >
      <svg
        className="absolute inset-0 size-full"
        viewBox={`0 0 ${PANE.width} ${PANE.height}`}
        preserveAspectRatio="none"
        aria-hidden="true"
      >
        <rect width={PANE.width} height={PANE.height} className="fill-[var(--base)]" />
        {CONTOURS.map((radius) => (
          <ellipse
            key={radius}
            cx={PANE.width * 0.62}
            cy={PANE.height * 0.5}
            rx={radius}
            ry={radius * 0.64}
            className="fill-none stroke-[var(--rule)]"
            strokeWidth={1}
          />
        ))}
        <path
          d={routePath(coordinates)}
          className="fill-none stroke-[var(--accent)]"
          strokeWidth={3}
        />
      </svg>
      <div className="absolute top-3 left-3">{children}</div>
    </div>
  );
}
