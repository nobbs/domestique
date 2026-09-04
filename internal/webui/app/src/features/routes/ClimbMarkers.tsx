/**
 * The climbs, marked on the terrain rather than under it.
 *
 * A bracket track below the chart is a second row of furniture describing the
 * row above it, and it puts the mark a few pixels from the hump it names —
 * close enough to read, far enough that the eye has to travel. Drawn over the
 * plot, a col is bracketed across the ground it actually covers.
 *
 * An overlay rather than a change to the chart. `ElevationProfile` is shared
 * with the map and the window selection, and it has no business knowing about
 * climbs. The chart's own gutters are reserved by `PADDING`, so a layer inset
 * by them spans exactly the plotted terrain and needs no measurement.
 */

import type { Climb } from "../../lib/climbs";
import { PADDING } from "../../lib/plotAxis";

/** Where the brackets sit: just inside the top of the plotted area. */
const BRACKET_TOP = PADDING.top + 2;

export function ClimbMarkers({
  climbs,
  startMetres,
  endMetres,
  onSelect,
}: {
  climbs: Climb[];
  /** The shown window's edges — the whole route when nothing is zoomed. */
  startMetres: number;
  endMetres: number;
  /** Opens the shared window on one climb, as the sidebar's own rows do. */
  onSelect: (metres: number) => void;
}) {
  if (climbs.length === 0 || endMetres <= startMetres) {
    return null;
  }
  const span = endMetres - startMetres;

  return (
    <div
      className="pointer-events-none absolute inset-y-0"
      style={{ left: PADDING.left, right: PADDING.right }}
    >
      {climbs
        // Ordinal fixed to the full route before the window filters which brackets draw,
        // so a climb keeps the number it has in ClimbsSidebar however the chart is zoomed.
        .map((climb, index) => ({ climb, ordinal: index + 1 }))
        .filter(({ climb }) => climb.endMetres > startMetres && climb.startMetres < endMetres)
        .map(({ climb, ordinal }) => {
          const clampedStart = Math.max(climb.startMetres, startMetres);
          const clampedEnd = Math.min(climb.endMetres, endMetres);

          return (
            <button
              key={climb.startMetres}
              type="button"
              onClick={() => onSelect((climb.startMetres + climb.endMetres) / 2)}
              aria-label={`Climb ${ordinal}`}
              className="pointer-events-auto absolute rounded-full bg-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--accent)]"
              style={{
                top: BRACKET_TOP,
                height: 3,
                opacity: 0.55,
                left: `${((clampedStart - startMetres) / span) * 100}%`,
                width: `${((clampedEnd - clampedStart) / span) * 100}%`,
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
                {ordinal}
              </span>
            </button>
          );
        })}
    </div>
  );
}
