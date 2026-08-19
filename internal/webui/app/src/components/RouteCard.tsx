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
  /**
   * How many stages the source route has, when it has more than one.
   *
   * A stage of a split route is only half an answer on its own: the title names
   * the route it came from, but nothing in it says whether this is the first of
   * three or the last. Absent for a route with a single stage, where there is no
   * position to state and "Stage 1 of 1" would be noise on every card.
   */
  stageCount?: number | undefined;
}

export function RouteCard({ stage, href, preview, stageCount }: RouteCardProps) {
  return (
    <Link to={href} className="route-card">
      <span className="route-card__preview">{preview}</span>
      <span className="route-card__body">
        <span className="route-card__title">{stage.title}</span>
        {stageCount !== undefined && stageCount > 1 ? (
          <span className="route-card__stage">
            Stage {stage.stageOrder} of {stageCount}
          </span>
        ) : null}
        <span className="route-card__meta">
          <span>{formatDistance(stage.distanceMetres)}</span>
          {/*
           * Each figure says in words what its symbol stands for. The symbol
           * alone means nothing to a screen reader, and the tooltip that used
           * to explain it was reachable by pointer and by nothing else — no
           * keyboard, no finger — so the meaning of two of the three figures on
           * every card was hover-only. The words are taken out of the layout
           * rather than out of the document; the symbol stays for the eye.
           */}
          <span title="Total climbing">
            <span aria-hidden="true">↑ </span>
            <span className="visually-hidden">Total climbing </span>
            {formatAscent(stage.ascentMetres)}
          </span>
          <span title="Steepest sustained gradient">
            <span aria-hidden="true">⌃ </span>
            <span className="visually-hidden">Steepest sustained gradient </span>
            {formatGradient(stage.maxGradientPercent)}
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
