/**
 * The route's climbs, in the route's own panel, folded away.
 *
 * They belong with the route rather than with the dock's drawn lanes. A climb
 * is a thing about the route, like its distance and its ascent, and the panel
 * that says what the route is is the one already carrying those — which leaves
 * the dock as three lanes on one axis and nothing else.
 *
 * Folded by default, because the count and the worst of them is the deciding
 * fact and the rest is detail. The summary is the control: a line that already
 * says "seven climbs, the biggest thirteen kilometres at six percent" needs no
 * separate word to press.
 *
 * Climb data and nothing else. Everything the forecast knows about a col is
 * drawn along the band below — tile by tile, at every reading rather than only
 * at the seven a climb happens to fall on — and a weather figure repeated here
 * was the same reading in a worse place. It also made a table of one subject
 * into a table of two.
 */

import { IconChevronsRight } from "@tabler/icons-react";
import { useState } from "react";
import { Separator } from "../../components/ui/separator";
import type { Climb } from "../../lib/climbs";
import { formatAscent, formatDistance, formatGradient } from "../../lib/format";
import type { UnitSystem } from "../../lib/units";

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
 * The row's columns: ordinal, length, average gradient, steepest gradient,
 * ascent, and where it starts.
 *
 * Fixed tracks rather than a flex row, so a figure sits under the figure above
 * it. Laid out by content, each row starts its second fact wherever its first
 * one happened to end — and the list stops being readable down a column, which
 * is the only way anyone reads seven of anything. The four measured columns
 * are right-aligned for the same reason: it is the digits that have to line
 * up, not the words.
 *
 * The two gradients are why there is a header. One percentage in a row needs
 * no explaining; two adjacent ones are a riddle, and the answer — which is the
 * average and which the wall — is the whole reason for carrying both.
 */
const ROW_COLUMNS = "0.75rem 3.5rem 2.5rem 2.5rem 3.5rem minmax(0,1fr)";

const VISIBLE_ROWS = 6;

/**
 * The route's climbs in one line: how many, and the one that decides the day.
 *
 * What a reader choosing a route needs before opening the list, and what the
 * control to open it says.
 */
function climbSentence(climbs: Climb[], unitSystem: UnitSystem): string | null {
  const biggest = climbs.reduce<Climb | null>(
    (worst, climb) => (worst === null || climb.ascentMetres > worst.ascentMetres ? climb : worst),
    null,
  );
  if (biggest === null) {
    return null;
  }

  return `${climbs.length} ${climbs.length === 1 ? "climb" : "climbs"} · biggest ${formatDistance(biggest.distanceMetres, unitSystem)} at ${formatGradient(biggest.averageGradePercent)}`;
}

export function ClimbsTable({
  climbs,
  unitSystem,
  onSelect,
}: {
  climbs: Climb[];
  unitSystem: UnitSystem;
  /** Opens the shared window on one climb, as the chart's own brackets do. */
  onSelect: (climb: Climb) => void;
}) {
  const [open, setOpen] = useState(false);
  const summary = climbSentence(climbs, unitSystem);

  if (summary === null) {
    return null;
  }

  return (
    <section>
      {/* Divides the card, not the route: this is chrome, not structure. */}
      <Separator className="mb-2" />
      <h3>
        <button
          type="button"
          aria-expanded={open}
          // The visible line is the summary, which says what the route has
          // rather than what the control does — so the name says the second.
          aria-label={`${open ? "Hide" : "Show"} ${climbs.length} ${climbs.length === 1 ? "climb" : "climbs"}`}
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
        <>
          {/*
           * Outside the scroller, so the names stay put while the climbs move
           * under them — a header that scrolls away is a header that is absent
           * exactly when a reader has lost track of which column is which.
           */}
          <div
            className="mt-1.5 grid gap-2 px-1.5 text-[10px] leading-none text-[var(--ink-2)]"
            style={{ gridTemplateColumns: ROW_COLUMNS }}
            aria-hidden="true"
          >
            <span />
            <span className="text-right">Length</span>
            <span className="text-right">Avg</span>
            <span className="text-right">Max</span>
            <span className="text-right">Ascent</span>
            <span className="text-right">Starts</span>
          </div>
          <ol
            className="mt-1 snap-y snap-mandatory overflow-y-auto"
            style={{ maxHeight: ROW_HEIGHT * VISIBLE_ROWS }}
          >
            {climbs.map((climb, index) => {
              return (
                <li key={climb.startMetres} className="snap-start" style={{ height: ROW_HEIGHT }}>
                  <button
                    type="button"
                    onClick={() => onSelect(climb)}
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
                      {formatGradient(climb.maxGradePercent)}
                    </span>
                    <span className="text-right text-xs text-[var(--ink-2)] tabular-nums">
                      {formatAscent(climb.ascentMetres, unitSystem)}
                    </span>
                    <span className="truncate text-right text-[11px] text-[var(--ink-2)] tabular-nums">
                      {/* No "from": the column is called Starts. */}
                      {formatDistance(climb.startMetres, unitSystem)}
                    </span>
                  </button>
                </li>
              );
            })}
          </ol>
        </>
      )}
    </section>
  );
}
