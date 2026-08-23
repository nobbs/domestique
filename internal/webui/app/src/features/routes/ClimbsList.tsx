/**
 * The route's sustained climbs, each one a control that opens the same
 * shared window the chart's own drag-to-zoom gesture opens — so a climb
 * picked from this list reads on the map and the chart exactly as one picked
 * by hand would.
 *
 * Nothing is rendered for a route with none: a list titled "Climbs" over an
 * empty line would ask a question about a route that has already answered it.
 */

import type { Climb } from "../../lib/climbs";
import { formatAscent, formatDistance, formatGradient } from "../../lib/format";
import type { UnitSystem } from "../../lib/units";

export interface ClimbsListProps {
  climbs: Climb[];
  onSelect: (climb: Climb) => void;
  unitSystem: UnitSystem;
}

export function ClimbsList({ climbs, onSelect, unitSystem }: ClimbsListProps) {
  if (climbs.length === 0) {
    return null;
  }

  return (
    <div className="route-panel__climbs">
      <h3 className="route-panel__climbs-heading">Climbs</h3>
      <ol className="route-panel__climbs-list">
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
    </div>
  );
}
