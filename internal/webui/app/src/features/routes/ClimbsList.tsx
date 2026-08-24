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
    <section className="route-panel__climbs" aria-labelledby="climbs-heading">
      <h3 className="route-panel__climbs-heading" id="climbs-heading">
        <button
          className="route-panel__climbs-toggle"
          type="button"
          aria-expanded={expanded}
          aria-label={`${expanded ? "Hide" : "Show"} ${climbs.length} ${climbs.length === 1 ? "climb" : "climbs"}`}
          {...(expanded ? { "aria-controls": "climbs-list" } : {})}
          onClick={() => setExpanded((current) => !current)}
        >
          <span>Climbs</span>
          <span className="route-panel__climbs-count">{climbs.length}</span>
          <IconChevronsRight
            className="route-panel__climbs-chevron"
            aria-hidden="true"
            size={16}
            stroke={2}
          />
        </button>
      </h3>
      {expanded ? (
        <ol className="route-panel__climbs-list" id="climbs-list">
          {climbs.map((climb) => (
            <li key={climb.startMetres}>
              <button className="route-panel__climb" type="button" onClick={() => onSelect(climb)}>
                <span className="route-panel__climb-distance">
                  {formatDistance(climb.distanceMetres, unitSystem)}
                </span>
                <span className="route-panel__climb-figures">
                  {formatAscent(climb.ascentMetres, unitSystem)} · avg{" "}
                  {formatGradient(climb.averageGradePercent)} · max{" "}
                  {formatGradient(climb.maxGradePercent)}
                </span>
              </button>
            </li>
          ))}
        </ol>
      ) : null}
    </section>
  );
}
