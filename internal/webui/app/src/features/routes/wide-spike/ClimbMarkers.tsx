/**
 * The climbs, marked on the terrain rather than under it.
 *
 * A bracket track below the chart is a second row of furniture describing the
 * row above it, and it puts the mark a few pixels from the hump it names —
 * close enough to read, far enough that the eye has to travel. Drawn over the
 * plot, a col is bracketed across the ground it actually covers.
 *
 * An overlay rather than a change to the chart. `ElevationProfile` is the real
 * instrument and is shared with the route card, the map and the window
 * selection; a spike has no business growing it a climbs feature to try one
 * out. The chart's own gutters are reserved by `PADDING`, so a layer inset by
 * them spans exactly the plotted terrain and needs no measurement of its own.
 */

import type { Climb } from "../../../lib/climbs";
import { PADDING } from "../../../lib/plotAxis";

/** Where the brackets sit: just inside the top of the plotted area. */
const BRACKET_TOP = PADDING.top + 2;

export function ClimbMarkers({
  climbs,
  totalMetres,
  onSelect,
}: {
  climbs: Climb[];
  totalMetres: number;
  /** Opens the shared window on one climb, as the sidebar's own rows do. */
  onSelect: (metres: number) => void;
}) {
  if (climbs.length === 0 || totalMetres <= 0) {
    return null;
  }

  return (
    <div
      className="pointer-events-none absolute inset-y-0"
      style={{ left: PADDING.left, right: PADDING.right }}
    >
      {climbs.map((climb, index) => (
        <button
          key={climb.startMetres}
          type="button"
          onClick={() => onSelect((climb.startMetres + climb.endMetres) / 2)}
          aria-label={`Climb ${index + 1}`}
          className="pointer-events-auto absolute rounded-full bg-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--accent)]"
          style={{
            top: BRACKET_TOP,
            height: 3,
            opacity: 0.55,
            left: `${(climb.startMetres / totalMetres) * 100}%`,
            width: `${((climb.endMetres - climb.startMetres) / totalMetres) * 100}%`,
          }}
        >
          {/*
           * The ordinal hangs below its bracket rather than above it. Above,
           * it lands in the chart's top gutter, which is the room reserved so
           * that a summit touching the ceiling is not clipped — and on a route
           * whose big col is also its highest point, that is exactly where the
           * terrain is.
           */}
          <span className="absolute top-1 left-0 text-[10px] leading-none font-semibold text-[var(--ink-2)] tabular-nums">
            {index + 1}
          </span>
        </button>
      ))}
    </div>
  );
}
