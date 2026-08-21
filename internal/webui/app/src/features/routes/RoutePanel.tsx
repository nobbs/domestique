/**
 * One route, in the panel the search was in.
 *
 * Not a page. The map is the library and the library is the page, so opening a
 * route swaps what the top-left panel is holding rather than navigating away
 * from the ground the reader is looking at: the same route stays drawn, the
 * same camera moves to it, and the way back is one control at the top of this
 * panel.
 *
 * Read-only by design. There are deliberately no editing affordances here, and
 * the service writes nothing back to VeloPlanner — the two quiet actions at the
 * foot either leave for the provider or ask this service to work the route out
 * again.
 */

import type { Route } from "../../api/types";
import { RouteKey } from "../../components/RouteKey";
import { SourceRouteLink } from "../../components/SourceRouteLink";
import { formatAscent, formatDistance, formatElevation, formatGradient } from "../../lib/format";
import type { Highlight } from "../../lib/highlight";
import type { SurfaceSummary } from "../../lib/surface";
import { ReprocessButton } from "./ReprocessButton";

export interface RoutePanelProps {
  route: Route;
  /** The whole route's highest point, or null where there is no usable profile. */
  highestMetres: number | null;
  /**
   * Where the route is, and when it was read: the panel's second line.
   *
   * Composed by the page, because two of the three things it can say — the read
   * time and whether the accounts hold the library — are facts about the sync
   * rather than about this route.
   */
  subtitle: string;
  /** Null for a route nobody has classified, which the key says in words. */
  surface: SurfaceSummary | null;
  surfaceAbsence: string;
  /** The bands this route actually has, gentlest first. */
  bands: number[];
  highlight: Highlight | null;
  onHighlightChange: (highlight: Highlight | null) => void;
  /** Puts the route away and gives the search pill back. */
  onClose: () => void;
  /** The provider's web application, for the way back to the source route. */
  sourceBaseUrl: string | undefined;
}

export function RoutePanel({
  route,
  highestMetres,
  subtitle,
  surface,
  surfaceAbsence,
  bands,
  highlight,
  onHighlightChange,
  onClose,
  sourceBaseUrl,
}: RoutePanelProps) {
  return (
    <section className="panel route-panel" aria-label={route.title}>
      {/*
       * The way back, and the only thing above the name: this panel replaced
       * the search, so a reader who opened a route by accident has to be able
       * to see how to get the search back without reading the route first.
       */}
      <button className="route-panel__back" type="button" onClick={onClose}>
        ← Back to search
      </button>
      <h2 className="route-panel__title">{route.title}</h2>
      {subtitle === "" ? null : <p className="route-panel__subtitle">{subtitle}</p>}
      <dl className="route-panel__figures">
        <div>
          <dt>Distance</dt>
          <dd>{formatDistance(route.distanceMetres)}</dd>
        </div>
        <div>
          <dt>Climbing</dt>
          <dd>{formatAscent(route.ascentMetres)}</dd>
        </div>
        <div>
          <dt>Max gradient</dt>
          <dd>{formatGradient(route.maxGradientPercent)}</dd>
        </div>
        <div>
          <dt>Highest</dt>
          {/*
           * Of the whole route, never of the stretch on show: it is one of the
           * four things this route is, and a figure that changed while a reader
           * dragged across the chart would be a fifth instrument.
           */}
          <dd>{highestMetres === null ? "—" : formatElevation(highestMetres)}</dd>
        </div>
      </dl>
      {/*
       * The two mixes as chips, and pressing one lights that ground on both the
       * map and the chart. They sit here rather than under the plot because the
       * plot is a panel of its own now, and a key that travelled with it would
       * disappear whenever the reader collapsed the chart to look at the map.
       */}
      <RouteKey
        surface={surface}
        surfaceAbsence={surfaceAbsence}
        bands={bands}
        highlight={highlight}
        onHighlightChange={onHighlightChange}
      />
      <div className="route-panel__actions">
        <SourceRouteLink baseUrl={sourceBaseUrl} routeId={route.routeId} />
        <ReprocessButton routeId={route.routeId} stageOrder={route.stageOrder} />
      </div>
    </section>
  );
}
