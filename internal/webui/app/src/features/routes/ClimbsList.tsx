/**
 * The route's sustained climbs, each one a control that opens the same
 * shared window the chart's own drag-to-zoom gesture opens — so a climb
 * picked from this list reads on the map and the chart exactly as one picked
 * by hand would.
 *
 * Nothing is rendered for a route with none: a list titled "Climbs" over an
 * empty line would ask a question about a route that has already answered it.
 */

import { IconChevronsRight } from "@tabler/icons-react";
import { useState } from "react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import type { Climb } from "../../lib/climbs";
import { formatAscent, formatDistance, formatGradient } from "../../lib/format";
import type { UnitSystem } from "../../lib/units";

export interface ClimbsListProps {
  climbs: Climb[];
  onSelect: (climb: Climb) => void;
  unitSystem: UnitSystem;
}

export function ClimbsList({ climbs, onSelect, unitSystem }: ClimbsListProps) {
  const [expanded, setExpanded] = useState(false);

  if (climbs.length === 0) {
    return null;
  }

  return (
    <Collapsible
      open={expanded}
      onOpenChange={setExpanded}
      className="border-t border-[var(--rule)] pt-4"
      render={<section aria-labelledby="climbs-heading" />}
    >
      <h3 id="climbs-heading">
        <CollapsibleTrigger
          className="flex w-full items-center gap-2 text-left font-semibold focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--accent)]"
          aria-label={`${expanded ? "Hide" : "Show"} ${climbs.length} ${climbs.length === 1 ? "climb" : "climbs"}`}
        >
          <span>Climbs</span>
          <span className="rounded-full bg-[var(--base)] px-1.5 py-0.5 text-xs text-[var(--ink-2)]">
            {climbs.length}
          </span>
          <IconChevronsRight
            className={expanded ? "rotate-90 transition-transform" : "transition-transform"}
            aria-hidden="true"
            size={16}
            stroke={2}
          />
        </CollapsibleTrigger>
      </h3>
      <CollapsibleContent render={<ol className="mt-2 grid gap-1" />}>
        {climbs.map((climb) => (
          <li key={climb.startMetres}>
            <button
              className="grid w-full grid-cols-[auto_1fr] gap-x-3 rounded-lg p-2 text-left text-sm hover:bg-[var(--base)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--accent)]"
              type="button"
              onClick={() => onSelect(climb)}
            >
              <span className="font-semibold tabular-nums">
                {formatDistance(climb.distanceMetres, unitSystem)}
              </span>
              <span className="text-[var(--ink-2)]">
                {formatAscent(climb.ascentMetres, unitSystem)} · avg{" "}
                {formatGradient(climb.averageGradePercent)} · max{" "}
                {formatGradient(climb.maxGradePercent)}
              </span>
            </button>
          </li>
        ))}
      </CollapsibleContent>
    </Collapsible>
  );
}
