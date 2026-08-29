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
import { SURFACE_KINDS } from "../../api/types";
import { Button } from "../../components/Button";
import { Checkbox } from "../../components/ui/checkbox";
import { FieldLabel } from "../../components/ui/field";
import { Popover, PopoverContent, PopoverTrigger } from "../../components/ui/popover";
import type { LibraryFilters } from "../../lib/filters";
import { EMPTY_FILTERS, hasActiveFilters } from "../../lib/filters";
import { SURFACE_STYLES } from "../../lib/surface";
import { RangeRow } from "./RangeRow";

const FILTER_PANEL_ID = "library-filter-panel";

export interface FilterPanelProps {
  filters: LibraryFilters;
  onFiltersChange: (next: LibraryFilters) => void;
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
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
            // Rounded on the way in as well as out, to the same 0.1 km step: a value
            // typed with finer precision would otherwise keep filtering at that precision
            // while the field shows a rounder number.
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
            variant="outline"
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
