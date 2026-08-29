/**
 * The route's climbs, in the route's own panel, folded away.
 *
 * They were a column beside the lanes, which put them with the *drawn*
 * readings — and they are not one. A climb is a thing about the route, like
 * its distance and its ascent, and the panel that says what the route is is
 * the one already carrying those. Moving them there leaves the dock as three
 * lanes on one axis and nothing else, which is all it ever claimed to be.
 *
 * Folded by default, because the count and the worst of them is the deciding
 * fact and the rest is detail — the same reason `ClimbsList` folds in the
 * panel this is a sketch for. The summary is the control: a line that already
 * says "seven climbs, the biggest thirteen kilometres at six percent" needs no
 * separate word to press.
 *
 * Each row still carries the weather waiting on that col, which is the join
 * the split alternative found and the one thing here neither panel could say
 * on its own.
 */

import { IconChevronsRight } from "@tabler/icons-react";
import { useState } from "react";
import type { Cell } from "../../../components/route/forecast-spike/cells";
import type { Climb } from "../../../lib/climbs";
import { formatAscent, formatDistance, formatGradient } from "../../../lib/format";
import type { UnitSystem } from "../../../lib/units";
import { weatherIcon } from "../../../lib/weather";
import { climbSentence } from "../panel-spike/shared";
import { cellAt, clockAt } from "./shared";

/**
 * One row's height, and how many are shown.
 *
 * Fixed rather than left to the content so the list can scroll in whole
 * climbs. A scroll container sized by whatever happens to fit leaves a row
 * sliced along the bottom edge, which reads as a rendering fault rather than
 * as more list — and a half-drawn climb is a half-drawn figure.
 *
 * With every row the same height and the container an exact multiple of it,
 * snapping has somewhere to land, and the list always rests showing whole
 * climbs.
 *
 * Six of them, on one line each. A climb is five short facts — how long, how
 * steep, how much it climbs, where it starts, and the weather waiting on it —
 * and the card is wide enough to say all five across, which halves the row and
 * lets the list show most routes whole without scrolling at all.
 */
const ROW_HEIGHT = 28;

/**
 * The row's columns: ordinal, length, gradient, ascent, then where and when,
 * then the weather.
 *
 * Fixed tracks rather than a flex row, so a figure sits under the figure above
 * it. Laid out by content, each row starts its second fact wherever its first
 * one happened to end — and the list stops being readable down a column, which
 * is the only way anyone reads seven of anything. The four measured columns
 * are right-aligned for the same reason: it is the digits that have to line
 * up, not the words.
 */
const ROW_COLUMNS = "0.75rem 3.5rem 2.5rem 3.5rem minmax(0,1fr) auto";
const VISIBLE_ROWS = 6;

export function ClimbsSection({
  climbs,
  cells,
  unitSystem,
  onSelect,
}: {
  climbs: Climb[];
  cells: Cell[];
  unitSystem: UnitSystem;
  /** Opens the shared window on one climb, as the chart's own brackets do. */
  onSelect: (metres: number) => void;
}) {
  const [open, setOpen] = useState(false);
  const summary = climbSentence(climbs, unitSystem);

  if (summary === null) {
    return null;
  }

  return (
    <section className="border-t border-[var(--rule)] pt-2">
      <h3>
        <button
          type="button"
          aria-expanded={open}
          onClick={() => setOpen(!open)}
          className="flex w-full items-center gap-1 text-left text-xs text-[var(--ink-2)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--accent)]"
        >
          <IconChevronsRight
            size={12}
            stroke={2}
            aria-hidden="true"
            className={
              open ? "shrink-0 rotate-90 transition-transform" : "shrink-0 transition-transform"
            }
          />
          {summary}
        </button>
      </h3>
      {!open ? null : (
        // Capped, for the reason the dock's own list is: seven climbs open to
        // a taller card than the gap between the menu bar and the dock, and a
        // card that grows past it slides its last climbs underneath. How much
        // map a reader loses should not depend on how lumpy their route is.
        //
        // `snap-mandatory` rather than `proximity`: this list is short and its
        // rows are uniform, so there is never a reading where landing between
        // two of them is what the reader meant.
        <ol
          className="mt-1.5 snap-y snap-mandatory overflow-y-auto"
          style={{ maxHeight: ROW_HEIGHT * VISIBLE_ROWS }}
        >
          {climbs.map((climb, index) => {
            // The middle of the climb rather than its foot: the weather a
            // rider remembers about a col is the weather on it.
            const middle = (climb.startMetres + climb.endMetres) / 2;
            const cell = cellAt(cells, middle);
            const Glyph = cell ? weatherIcon(cell.point.weatherCode) : null;

            return (
              <li key={climb.startMetres} className="snap-start" style={{ height: ROW_HEIGHT }}>
                <button
                  type="button"
                  onClick={() => onSelect(middle)}
                  className="grid size-full items-center gap-2 rounded-lg px-1.5 text-left hover:bg-[var(--base)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--accent)]"
                  style={{ gridTemplateColumns: ROW_COLUMNS }}
                >
                  <span className="text-right text-xs font-semibold tabular-nums">
                    {/* The ordinal the chart's own bracket carries. */}
                    {index + 1}
                  </span>
                  <span className="text-right text-xs tabular-nums">
                    {formatDistance(climb.distanceMetres, unitSystem)}
                  </span>
                  <span className="text-right text-xs tabular-nums">
                    {formatGradient(climb.averageGradePercent)}
                  </span>
                  <span className="text-right text-xs text-[var(--ink-2)] tabular-nums">
                    {formatAscent(climb.ascentMetres, unitSystem)}
                  </span>
                  <span className="truncate text-[11px] text-[var(--ink-2)] tabular-nums">
                    from {formatDistance(climb.startMetres, unitSystem)}
                    {cell === null ? null : ` · ${clockAt(cell.sample.arrivalAt)}`}
                  </span>
                  {cell === null ? null : (
                    <span className="flex items-center justify-end gap-1 text-xs tabular-nums">
                      {Glyph === null ? null : <Glyph size={14} stroke={1.8} aria-hidden="true" />}
                      <span className="font-semibold">
                        {Math.round(cell.point.temperatureCelsius)}°
                      </span>
                    </span>
                  )}
                </button>
              </li>
            );
          })}
        </ol>
      )}
    </section>
  );
}
