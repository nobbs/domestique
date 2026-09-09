/**
 * The range filters in view beside the table; below the breakpoint the table
 * itself folds at, the same sliders fold behind `FilterPanel`'s toggle.
 */

import { IconAdjustmentsHorizontal } from "@tabler/icons-react";
import { useMemo } from "react";
import type { Route } from "../../api/types";
import { Button } from "../../components/Button";
import { RangeSlider } from "../../components/RangeSlider";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "../../components/ui/collapsible";
import { domainOf } from "../../lib/domain";
import type { LibraryFilters } from "../../lib/filters";
import { EMPTY_FILTERS, hasActiveFilters } from "../../lib/filters";
import { formatMovingTime } from "../../lib/format";

export interface CatalogueFiltersProps {
  /** The whole library, whose distribution each slider draws over its track. */
  library: Route[];
  filters: LibraryFilters;
  onFiltersChange: (next: LibraryFilters) => void;
  /** Folds the sliders behind a toggle; above the breakpoint they stay open. */
  narrow: boolean;
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
}

export function CatalogueFilters({
  library,
  filters,
  onFiltersChange,
  narrow,
  expanded,
  onExpandedChange,
}: CatalogueFiltersProps) {
  const active = hasActiveFilters(filters);
  // By library only: the panel re-renders on every search keystroke.
  const measures = useMemo(() => {
    const distances = library.map((route) => route.distanceMetres);
    const ascents = library.map((route) => route.ascentMetres);
    const durations = library.map((route) => route.movingSeconds ?? 0);

    return {
      distances,
      ascents,
      durations,
      distance: domainOf(distances, [1_000, 2_000, 5_000, 10_000]),
      ascent: domainOf(ascents, [10, 20, 50, 100, 200]),
      duration: domainOf(durations, [5 * 60, 10 * 60, 15 * 60, 30 * 60]),
    };
  }, [library]);
  const { distances, ascents, durations, distance, ascent, duration } = measures;

  const sliders = (
    <div className="flex flex-wrap items-end gap-x-6 gap-y-4">
      <div className="min-w-44 flex-1">
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
      </div>
      <div className="min-w-44 flex-1">
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
      </div>
      <div className="min-w-44 flex-1">
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
      </div>
      <Button variant="outline" disabled={!active} onClick={() => onFiltersChange(EMPTY_FILTERS)}>
        Clear filters
      </Button>
    </div>
  );

  if (!narrow) {
    return (
      <div className="rounded-lg border border-[var(--rule)] bg-[var(--panel)] p-3">{sliders}</div>
    );
  }

  return (
    <Collapsible open={expanded} onOpenChange={onExpandedChange}>
      <CollapsibleTrigger
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
      <CollapsibleContent className="pt-3">{sliders}</CollapsibleContent>
    </Collapsible>
  );
}
