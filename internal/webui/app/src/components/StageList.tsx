/**
 * A selectable list of source stages. Presentational: it takes stages and a
 * link builder, and knows nothing about how they were fetched.
 */

import { NavLink } from "react-router";
import type { Stage } from "../api/types";
import { stageKey } from "../api/types";
import { formatCount, formatDistance } from "../lib/format";

export interface StageListProps {
  stages: Stage[];
  hrefFor: (stage: Stage) => string;
}

export function StageList({ stages, hrefFor }: StageListProps) {
  if (stages.length === 0) {
    return (
      <p className="stage-list__empty">
        No stages have been synced yet. They appear after the first successful run.
      </p>
    );
  }

  return (
    <ul className="stage-list">
      {stages.map((stage) => (
        <li key={stageKey(stage)}>
          <NavLink
            to={hrefFor(stage)}
            className={({ isActive }) =>
              isActive ? "stage-list__item stage-list__item--active" : "stage-list__item"
            }
          >
            <span className="stage-list__title">{stage.title}</span>
            <span className="stage-list__meta">
              {formatDistance(stage.distanceMetres)}
              {" · "}
              {formatCount(stage.pointCount, "point")}
            </span>
          </NavLink>
        </li>
      ))}
    </ul>
  );
}
