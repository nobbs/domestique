/**
 * The catalogue's head.
 *
 * Folding four measured columns into two cells would have taken four sort
 * headings with them, and ranking is the whole reason the catalogue exists. So
 * each cell names its own measures and those are the controls: the ride cell
 * ranks by distance or by time, the climbing cell by ascent or by steepest
 * gradient.
 *
 * The cells carry no group word. "Ride" and "Climbing" named nothing the
 * measures under them did not already name, and an unpressable heading beside
 * two pressable ones only muddles which of them is the control.
 */

import { IconArrowDown, IconArrowUp } from "@tabler/icons-react";
import type { CatalogueView, SortColumn } from "../../lib/catalogue";
import { sortColumn } from "../../lib/catalogue";

/**
 * Which measures each cell ranks by, in the order they are drawn.
 *
 * The columns are named, not their labels: those come from `SORT_COLUMNS`, so
 * the control and the sentence that describes the order are spelled from one
 * table rather than two that can drift.
 */
export const GROUPED_COLUMNS: ReadonlyArray<{
  readonly cell: string;
  readonly columns: readonly SortColumn[];
}> = [
  { cell: "ride", columns: ["distance", "movingTime"] },
  { cell: "climbing", columns: ["ascent", "gradient"] },
];

function SortButton({
  column,
  view,
  onSort,
}: {
  column: SortColumn;
  view: CatalogueView;
  onSort: (column: SortColumn) => void;
}) {
  const active = view.sort === column;
  const label = sortColumn(column)?.short ?? column;

  return (
    <button
      type="button"
      onClick={() => onSort(column)}
      aria-pressed={active}
      className={`inline-flex items-center gap-0.5 rounded px-1 py-0.5 text-xs hover:bg-[var(--base)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--accent)] ${
        active ? "font-semibold text-[var(--ink)]" : "font-normal text-[var(--ink-2)]"
      }`}
    >
      {label}
      {active ? (
        view.direction === "asc" ? (
          <IconArrowUp size={12} stroke={2.2} aria-hidden="true" />
        ) : (
          <IconArrowDown size={12} stroke={2.2} aria-hidden="true" />
        )
      ) : null}
    </button>
  );
}

export interface CatalogueHeaderProps {
  view: CatalogueView;
  onSort: (column: SortColumn) => void;
  /** The route cell's own heading, which ranks by name. */
  children: React.ReactNode;
}

export function CatalogueHeader({ view, onSort, children }: CatalogueHeaderProps) {
  return (
    <tr className="border-[var(--rule)] border-b bg-[var(--base)]">
      {/*
       * The shapes have no heading: the column ranks by nothing and repeats the
       * route the cell beside it names. It carries the same border width the
       * rows spend on their change marker so the two line up.
       */}
      <th scope="col" className="border-l-[3px] border-l-transparent">
        <span className="sr-only">Shape</span>
      </th>
      {children}
      {GROUPED_COLUMNS.map(({ cell, columns }) => {
        // The cell is sorted when any of its measures is, which is what a
        // reader moving by column needs to hear without landing on a control.
        const sorted = columns.includes(view.sort);

        return (
          <th
            key={cell}
            scope="col"
            aria-sort={sorted ? (view.direction === "asc" ? "ascending" : "descending") : "none"}
            className="px-3 py-2 text-right align-bottom"
          >
            <span className="flex justify-end gap-2">
              {columns.map((column) => (
                <SortButton key={column} column={column} view={view} onSort={onSort} />
              ))}
            </span>
          </th>
        );
      })}
    </tr>
  );
}
