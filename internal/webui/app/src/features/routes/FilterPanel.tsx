/**
 * Narrowing the library shown, by what a route measures rather than what it
 * is called.
 *
 * Folded away until asked for, the same way the map's basemap choice is: a
 * filter nobody has set is a fact worth confirming rather than a control
 * worth spending room on beside the search field. Expansion is held by the
 * caller for the same reason it is there — see `BasemapPicker`.
 */

import { SURFACE_KINDS } from "../../api/types";
import { Button } from "../../components/Button";
import type { LibraryFilters, NumericRange } from "../../lib/filters";
import { EMPTY_FILTERS, hasActiveFilters } from "../../lib/filters";
import { SURFACE_STYLES } from "../../lib/surface";

const FILTER_PANEL_ID = "library-filter-panel";

export interface FilterPanelProps {
  filters: LibraryFilters;
  onFiltersChange: (next: LibraryFilters) => void;
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
}

/** One field's text, `null` for an empty — and so unbounded — side. */
function parseBound(value: string): number | null {
  if (value.trim() === "") {
    return null;
  }
  const parsed = Number(value);

  return Number.isFinite(parsed) ? parsed : null;
}

interface RangeRowProps {
  legend: string;
  unit: string;
  range: NumericRange;
  onChange: (next: NumericRange) => void;
  /** Stored-unit (metres, percent) to and from what the field shows. */
  toDisplay?: (stored: number) => number;
  toStored?: (display: number) => number;
}

/** One metric's min and max, both inclusive, both optional. */
function RangeRow({
  legend,
  unit,
  range,
  onChange,
  toDisplay = (value) => value,
  toStored = (value) => value,
}: RangeRowProps) {
  const bound = (side: "min" | "max") => (
    <label className="filter-panel__bound">
      {side === "min" ? "Min" : "Max"}
      <input
        type="number"
        inputMode="decimal"
        value={range[side] === null ? "" : toDisplay(range[side])}
        onChange={(event) => {
          const parsed = parseBound(event.target.value);
          onChange({ ...range, [side]: parsed === null ? null : toStored(parsed) });
        }}
      />
    </label>
  );

  return (
    <fieldset className="filter-panel__range">
      <legend>
        {legend} ({unit})
      </legend>
      {bound("min")}
      {bound("max")}
    </fieldset>
  );
}

export function FilterPanel({
  filters,
  onFiltersChange,
  expanded,
  onExpandedChange,
}: FilterPanelProps) {
  const active = hasActiveFilters(filters);

  return (
    <div className="filter-panel">
      <button
        className="filter-panel__toggle"
        type="button"
        aria-expanded={expanded}
        // The mark says "filters are set" to anyone who can see it; the name
        // says so for anyone who cannot, the same split `BasemapPicker` uses.
        aria-label={
          expanded
            ? "Hide the library filters"
            : active
              ? "Show the library filters — filters are active"
              : "Show the library filters"
        }
        {...(expanded ? { "aria-controls": FILTER_PANEL_ID } : {})}
        onClick={() => onExpandedChange(!expanded)}
      >
        Filters{active ? " •" : ""}
      </button>
      {expanded ? (
        <div className="filter-panel__body" id={FILTER_PANEL_ID}>
          <RangeRow
            legend="Distance"
            unit="km"
            range={filters.distanceMetres}
            onChange={(next) => onFiltersChange({ ...filters, distanceMetres: next })}
            toDisplay={(metres) => Math.round((metres / 1000) * 10) / 10}
            toStored={(km) => km * 1000}
          />
          <RangeRow
            legend="Ascent"
            unit="m"
            range={filters.ascentMetres}
            onChange={(next) => onFiltersChange({ ...filters, ascentMetres: next })}
          />
          <RangeRow
            legend="Max gradient"
            unit="%"
            range={filters.maxGradientPercent}
            onChange={(next) => onFiltersChange({ ...filters, maxGradientPercent: next })}
          />
          <fieldset className="filter-panel__surfaces">
            <legend>Surface</legend>
            {SURFACE_KINDS.map((kind) => (
              <label className="filter-panel__surface" key={kind}>
                <input
                  type="checkbox"
                  checked={filters.surfaces.includes(kind)}
                  onChange={(event) => {
                    const surfaces = event.target.checked
                      ? [...filters.surfaces, kind]
                      : filters.surfaces.filter((selected) => selected !== kind);
                    onFiltersChange({ ...filters, surfaces });
                  }}
                />
                {SURFACE_STYLES[kind].label}
              </label>
            ))}
          </fieldset>
          <Button disabled={!active} onClick={() => onFiltersChange(EMPTY_FILTERS)}>
            Clear filters
          </Button>
        </div>
      ) : null}
    </div>
  );
}
