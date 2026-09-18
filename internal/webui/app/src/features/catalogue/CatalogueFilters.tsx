/** The range filters, as the rail's own card: one inset row per measure. */

import { IconAdjustmentsHorizontal } from "@tabler/icons-react";
import { useMemo } from "react";
import type { Route } from "../../api/types";
import { Panel } from "../../components/PanelHeading";
import { RangeSlider } from "../../components/RangeSlider";
import { librarySources, SourceChips } from "../../components/SourceChips";
import { domainOf } from "../../lib/domain";
import type { LibraryFilters } from "../../lib/filters";
import { EMPTY_FILTERS, hasActiveFilters } from "../../lib/filters";
import { formatMovingTime } from "../../lib/format";

export interface CatalogueFiltersProps {
  /** The whole library, whose distribution each slider draws over its track. */
  library: Route[];
  filters: LibraryFilters;
  onFiltersChange: (next: LibraryFilters) => void;
}

export function CatalogueFilters({ library, filters, onFiltersChange }: CatalogueFiltersProps) {
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
  const sources = useMemo(() => librarySources(library), [library]);

  return (
    <Panel
      icon={<IconAdjustmentsHorizontal size={18} stroke={1.8} aria-hidden="true" />}
      title="Filters"
      aside={
        <button
          type="button"
          disabled={!active}
          onClick={() => onFiltersChange(EMPTY_FILTERS)}
          className="rounded-[9px] px-2.5 py-1 text-[var(--ink-2)] text-xs hover:bg-[var(--muted)] hover:text-[var(--ink)] disabled:opacity-40"
        >
          Clear
        </button>
      }
    >
      <div className="flex flex-col overflow-hidden rounded-[11px] bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)]">
        {/* One source leaves nothing to choose between. */}
        {sources.length > 1 ? (
          <div className="flex flex-col gap-2 border-[var(--panel)] border-b-2 px-3.5 py-3">
            <span className="font-semibold text-sm">Source</span>
            <SourceChips
              sources={sources}
              chosen={filters.providers}
              onChange={(providers) => onFiltersChange({ ...filters, providers })}
            />
          </div>
        ) : null}
        <div className="border-[var(--panel)] border-b-2 px-3.5 py-3 last:border-b-0">
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
        <div className="border-[var(--panel)] border-b-2 px-3.5 py-3 last:border-b-0">
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
        <div className="border-[var(--panel)] border-b-2 px-3.5 py-3 last:border-b-0">
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
      </div>
    </Panel>
  );
}
