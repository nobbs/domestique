/**
 * Narrowing the library shown, by what a route measures rather than what it
 * is called.
 *
 * Folded away until asked for, the same way the map's basemap choice is: a
 * filter nobody has set is a fact worth confirming rather than a control
 * worth spending room on beside the search field. Expansion is held by the
 * caller for the same reason it is there — see `BasemapPicker`.
 */

import { IconAdjustmentsHorizontal } from "@tabler/icons-react";
import { useId, useState } from "react";
import { SURFACE_KINDS } from "../../api/types";
import { Button } from "../../components/Button";
import { Checkbox } from "../../components/ui/checkbox";
import { FieldLabel } from "../../components/ui/field";
import { Input } from "../../components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "../../components/ui/popover";
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

interface BoundInputProps {
  label: string;
  /** The bound in its stored unit — metres, percent — not what the field shows. */
  stored: number | null;
  onChange: (stored: number | null) => void;
  toDisplay: (stored: number) => number;
  toStored: (display: number) => number;
}

function displayOf(stored: number | null, toDisplay: (stored: number) => number): string {
  return stored === null ? "" : String(toDisplay(stored));
}

/**
 * One bound, typed as free text rather than driven straight from the stored
 * number.
 *
 * A controlled field whose value is `toDisplay(stored)` on every keystroke
 * fights whatever was just typed: "1." parses to the same number as "1", so
 * a value re-derived from the parsed number drops the point before a second
 * digit can follow it, and typing a fraction becomes impossible. The text is
 * kept here instead, and is resynced from `stored` only when it no longer
 * matches this field's own last edit — the signal that the change came from
 * outside, such as Clear filters, rather than from what was just typed.
 *
 * `type="text"` rather than `type="number"`, deliberately: a number input's
 * own `.value` reports empty for "1." too, per the HTML value-sanitisation
 * algorithm — a partial decimal is not yet a complete floating-point number —
 * so the browser itself would hand back the empty string this component is
 * trying to stop reading. `inputMode="decimal"` still asks a touch keyboard
 * for digits and a point.
 */
function BoundInput({ label, stored, onChange, toDisplay, toStored }: BoundInputProps) {
  const id = useId();
  const [text, setText] = useState(() => displayOf(stored, toDisplay));
  const [lastStored, setLastStored] = useState(stored);

  if (stored !== lastStored) {
    setLastStored(stored);
    setText(displayOf(stored, toDisplay));
  }

  return (
    <label className="grid gap-1 text-xs text-[var(--ink-2)]" htmlFor={id}>
      <span>{label}</span>
      <Input
        id={id}
        className="bg-[var(--panel)]"
        type="text"
        inputMode="decimal"
        value={text}
        onChange={(event) => {
          const raw = event.target.value;
          setText(raw);
          const parsed = parseBound(raw);
          const next = parsed === null ? null : toStored(parsed);
          setLastStored(next);
          onChange(next);
        }}
      />
    </label>
  );
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
  return (
    <fieldset className="grid grid-cols-2 gap-2">
      <legend className="col-span-2 text-sm font-semibold">
        {legend} ({unit})
      </legend>
      <BoundInput
        label="Min"
        stored={range.min}
        onChange={(min) => onChange({ ...range, min })}
        toDisplay={toDisplay}
        toStored={toStored}
      />
      <BoundInput
        label="Max"
        stored={range.max}
        onChange={(max) => onChange({ ...range, max })}
        toDisplay={toDisplay}
        toStored={toStored}
      />
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
    <Popover open={expanded} onOpenChange={onExpandedChange}>
      <PopoverTrigger
        render={
          <Button
            variant="panel"
            icon={<IconAdjustmentsHorizontal stroke={1.6} />}
            active={active}
          />
        }
        // The mark says "filters are set" to anyone who can see it; the name
        // says so for anyone who cannot, the same split `BasemapPicker` uses.
        aria-label={
          expanded
            ? "Hide the library filters"
            : active
              ? "Show the library filters — filters are active"
              : "Show the library filters"
        }
      />
      <PopoverContent
        // Named for the same reason the basemap popup is: it is a dialog to a
        // screen reader, and an unnamed one says nothing about what it holds.
        aria-label="Library filters"
        className="w-[min(23rem,calc(100vw-1.5rem))] gap-4 bg-[var(--panel)] p-3 shadow-[var(--shadow)]"
        id={FILTER_PANEL_ID}
      >
        <div className="grid gap-4">
          <RangeRow
            legend="Distance"
            unit="km"
            range={filters.distanceMetres}
            onChange={(next) => onFiltersChange({ ...filters, distanceMetres: next })}
            // Rounded on the way in as well as the way out, to the same 0.1 km
            // step: a value typed with finer precision than the field ever
            // displays would otherwise keep filtering at that precision after
            // the panel folds and reopens, on a field that now shows a
            // rounder number than the one actually in force.
            toDisplay={(metres) => Math.round((metres / 1000) * 10) / 10}
            toStored={(km) => Math.round(km * 10) * 100}
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
          <fieldset className="grid gap-2">
            <legend className="text-sm font-semibold">Surface</legend>
            {SURFACE_KINDS.map((kind) => (
              <FieldLabel className="flex items-center gap-2 text-sm" key={kind}>
                <Checkbox
                  checked={filters.surfaces.includes(kind)}
                  onCheckedChange={(checked) => {
                    const surfaces = checked
                      ? [...filters.surfaces, kind]
                      : filters.surfaces.filter((selected) => selected !== kind);
                    onFiltersChange({ ...filters, surfaces });
                  }}
                />
                {SURFACE_STYLES[kind].label}
              </FieldLabel>
            ))}
          </fieldset>
          <Button
            variant="standard"
            disabled={!active}
            onClick={() => onFiltersChange(EMPTY_FILTERS)}
          >
            Clear filters
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}
