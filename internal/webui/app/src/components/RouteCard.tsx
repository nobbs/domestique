/**
 * One route stage as a small box in the library grid. Presentational: it takes
 * a preview to render and a destination, and knows nothing about fetching.
 */

import { Link } from "react-router";
import type { Stage } from "../api/types";
import { formatAscent, formatDistance, formatGradient } from "../lib/format";

export interface RouteCardProps {
  stage: Stage;
  href: string;
  preview: React.ReactNode;
}

export function RouteCard({ stage, href, preview }: RouteCardProps) {
  return (
    <Link to={href} className="route-card">
      <span className="route-card__preview">{preview}</span>
      <span className="route-card__body">
        <span className="route-card__title">{stage.title}</span>
        <span className="route-card__meta">
          <span>{formatDistance(stage.distanceMetres)}</span>
          <span title="Total climbing">↑ {formatAscent(stage.ascentMetres)}</span>
          <span title="Steepest sustained gradient">
            ⌃ {formatGradient(stage.maxGradientPercent)}
          </span>
        </span>
      </span>
    </Link>
  );
}

/** The grid the cards sit in. Its column count is handled entirely in CSS. */
export function RouteGrid({ children }: { children: React.ReactNode }) {
  return <ul className="route-grid">{children}</ul>;
}
