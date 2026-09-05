/**
 * Narrowing the library shown, by what a route measures rather than what it
 * is called. Folded away until asked for, the same way the basemap choice is.
 */

import { IconAdjustmentsHorizontal } from "@tabler/icons-react";
import type { Route } from "../../api/types";
import { Button } from "../../components/Button";
import { RangeSlider } from "../../components/RangeSlider";
import { Popover, PopoverContent, PopoverTrigger } from "../../components/ui/popover";
import { domainOf } from "../../lib/domain";
import type { LibraryFilters } from "../../lib/filters";
import { EMPTY_FILTERS, hasActiveFilters } from "../../lib/filters";
import { formatMovingTime } from "../../lib/format";

const FILTER_PANEL_ID = "library-filter-panel";

export interface FilterPanelProps {
  /** The whole library, whose distribution each slider draws over its track. */
  library: Route[];
  filters: LibraryFilters;
  onFiltersChange: (next: LibraryFilters) => void;
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
}

export function FilterPanel({
  library,
  filters,
  onFiltersChange,
  expanded,
  onExpandedChange,
}: FilterPanelProps) {
  const active = hasActiveFilters(filters);
  const distances = library.map((route) => route.distanceMetres);
  const ascents = library.map((route) => route.ascentMetres);
  const durations = library.map((route) => route.movingSeconds ?? 0);
  const distance = domainOf(distances, [1_000, 2_000, 5_000, 10_000]);
  const ascent = domainOf(ascents, [10, 20, 50, 100, 200]);
  const duration = domainOf(durations, [5 * 60, 10 * 60, 15 * 60, 30 * 60]);

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
            max={distance.max}
            step={distance.step}
            range={filters.distanceMetres}
            onChange={(next) => onFiltersChange({ ...filters, distanceMetres: next })}
            format={(metres) => `${metres / 1000} km`}
            values={distances}
          />
          <RangeSlider
            legend="Ascent"
            min={0}
            max={ascent.max}
            step={ascent.step}
            range={filters.ascentMetres}
            onChange={(next) => onFiltersChange({ ...filters, ascentMetres: next })}
            format={(metres) => `${metres} m`}
            values={ascents}
          />
          <RangeSlider
            legend="Duration"
            min={0}
            max={duration.max}
            step={duration.step}
            range={filters.movingSeconds}
            onChange={(next) => onFiltersChange({ ...filters, movingSeconds: next })}
            format={(seconds) => (seconds === 0 ? "0 min" : formatMovingTime(seconds))}
            values={durations}
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
