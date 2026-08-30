/**
 * The route's climbs, beside the chart that draws them.
 *
 * They belong on the same axis as everything else the dock holds: a climb is a
 * stretch of the route, and the column that says where it starts is only
 * meaningful against a distance the chart is already showing. `ClimbMarkers`
 * brackets each one across the ground it covers a few pixels away, so the
 * shape and its figures are finally in the same place.
 *
 * Beside rather than under, which is the whole reason this shape works. Under
 * the chart the table competes with the profile for the dock's height and
 * pushes the forecast off the foot; beside it, it costs width the full-width
 * dock has and height it does not.
 *
 * Folded it keeps its count, turned on its side. A rail is a poor thing to
 * read but a good thing to find, and what a reader folding this away wants
 * back is the chart's width rather than the memory of where the control went.
 *
 * Climb data and nothing else. Everything the forecast knows about a col is
 * drawn along the band below — tile by tile, at every reading rather than only
 * at the seven a climb happens to fall on — and a weather figure repeated here
 * was the same reading in a worse place.
 */

import { IconChevronRight, IconLayoutSidebarRightCollapse } from "@tabler/icons-react";
import type { Climb } from "../../lib/climbs";
import { formatAscent, formatDistance, formatGradient } from "../../lib/format";
import type { UnitSystem } from "../../lib/units";

/**
 * The row's columns: ordinal, length, average gradient, steepest gradient,
 * ascent, and where it starts.
 *
 * Fixed tracks rather than a flex row, so a figure sits under the figure above
 * it. Laid out by content, each row starts its second fact wherever its first
 * one happened to end — and the list stops being readable down a column, which
 * is the only way anyone reads seven of anything.
 *
 * The two gradients are why there is a header. One percentage in a row needs
 * no explaining; two adjacent ones are a riddle, and the answer — which is the
 * average and which the wall — is the whole reason for carrying both.
 */
const ROW_COLUMNS = "0.75rem 3.5rem 2.5rem 2.5rem 3.5rem minmax(0,1fr)";
const ROW_HEIGHT = 28;

/**
 * The route's climbs in one line: how many, and the one that decides the day.
 *
 * What a reader deciding whether to open the list needs before opening it.
 */
export function climbSentence(climbs: Climb[], unitSystem: UnitSystem): string | null {
  const biggest = climbs.reduce<Climb | null>(
    (worst, climb) => (worst === null || climb.ascentMetres > worst.ascentMetres ? climb : worst),
    null,
  );
  if (biggest === null) {
    return null;
  }

  return `biggest ${formatDistance(biggest.distanceMetres, unitSystem)} at ${formatGradient(biggest.averageGradePercent)}`;
}

export function ClimbsSidebar({
  climbs,
  open,
  onOpenChange,
  onSelect,
  unitSystem,
}: {
  climbs: Climb[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Opens the shared map/chart window on one climb, as the brackets do. */
  onSelect: (climb: Climb) => void;
  unitSystem: UnitSystem;
}) {
  const summary = climbSentence(climbs, unitSystem);
  // A route with no climbs has no sidebar: an empty column beside the chart
  // spends width saying that a flat route is flat.
  if (summary === null) {
    return null;
  }
  const count = `${climbs.length} ${climbs.length === 1 ? "climb" : "climbs"}`;

  if (!open) {
    return (
      <button
        type="button"
        aria-expanded={false}
        aria-label={`Show ${count}`}
        onClick={() => onOpenChange(true)}
        className="flex shrink-0 items-center gap-1.5 self-stretch rounded-lg border border-[var(--rule)] px-2 text-[11px] text-[var(--ink-2)] hover:bg-[var(--base)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--accent)]"
      >
        <IconLayoutSidebarRightCollapse size={14} stroke={2} aria-hidden="true" />
        <span className="rotate-180 [writing-mode:vertical-rl]">{count}</span>
      </button>
    );
  }

  return (
    <section className="w-80 shrink-0 self-stretch border-l border-[var(--rule)] pl-3">
      <h3>
        <button
          type="button"
          aria-expanded
          aria-label={`Hide ${count}`}
          onClick={() => onOpenChange(false)}
          className="flex w-full items-baseline gap-2 rounded py-0.5 text-left hover:bg-[var(--base)] focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--accent)]"
        >
          <span className="text-[10px] font-semibold tracking-[0.08em] text-[var(--ink-2)] uppercase">
            {count}
          </span>
          <span className="min-w-0 flex-1 truncate text-[11px] text-[var(--ink-2)]">{summary}</span>
          <IconChevronRight
            size={12}
            stroke={2}
            aria-hidden="true"
            className="shrink-0 rotate-90 text-[var(--ink-2)]"
          />
        </button>
      </h3>
      {/*
       * Outside the scroller, so the names stay put while the climbs move
       * under them — a header that scrolls away is a header that is absent
       * exactly when a reader has lost track of which column is which.
       */}
      <div
        className="mt-1 grid gap-2 px-1.5 text-[10px] leading-none text-[var(--ink-2)]"
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
      {/*
       * `snap-mandatory` rather than `proximity`: this list is short and its
       * rows are uniform, so there is never a reading where landing between two
       * of them is what the reader meant. It scrolls against the dock's own
       * height rather than a row count, because beside the chart that height is
       * whatever the chart came to.
       */}
      <ol className="mt-1 max-h-full snap-y snap-mandatory overflow-y-auto">
        {climbs.map((climb, index) => (
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
        ))}
      </ol>
    </section>
  );
}
