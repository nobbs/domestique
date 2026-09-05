/**
 * Narrowing the library shown, by what a route measures rather than what it
 * is called. Folded away until asked for, the same way the basemap choice is.
 */

import { IconAdjustmentsHorizontal } from "@tabler/icons-react";
import { Button } from "../../components/Button";
import { RangeSlider } from "../../components/RangeSlider";
import { Popover, PopoverContent, PopoverTrigger } from "../../components/ui/popover";
import type { LibraryFilters } from "../../lib/filters";
import { EMPTY_FILTERS, hasActiveFilters } from "../../lib/filters";
import { formatMovingTime } from "../../lib/format";

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
        aria-label="Library filters"
        className="w-[min(23rem,calc(100vw-1.5rem))] gap-4 bg-[var(--panel)] p-3 shadow-[var(--shadow)]"
        id={FILTER_PANEL_ID}
      >
        <div className="grid gap-5">
          <RangeSlider
            legend="Distance"
            min={0}
            max={300_000}
            step={5_000}
            range={filters.distanceMetres}
            onChange={(next) => onFiltersChange({ ...filters, distanceMetres: next })}
            format={(metres) => `${metres / 1000} km`}
          />
          <RangeSlider
            legend="Ascent"
            min={0}
            max={5_000}
            step={100}
            range={filters.ascentMetres}
            onChange={(next) => onFiltersChange({ ...filters, ascentMetres: next })}
            format={(metres) => `${metres} m`}
          />
          <RangeSlider
            legend="Duration"
            min={0}
            max={12 * 3600}
            step={15 * 60}
            range={filters.movingSeconds}
            onChange={(next) => onFiltersChange({ ...filters, movingSeconds: next })}
            format={(seconds) => (seconds === 0 ? "0 min" : formatMovingTime(seconds))}
          />
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
