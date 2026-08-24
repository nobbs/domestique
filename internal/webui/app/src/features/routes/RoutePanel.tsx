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

import { IconArrowLeft, IconX } from "@tabler/icons-react";
import type { Route } from "../../api/types";
import { RouteKey } from "../../components/RouteKey";
import { SourceRouteLink } from "../../components/SourceRouteLink";
import type { Climb } from "../../lib/climbs";
import { formatAscent, formatDistance, formatElevation, formatGradient } from "../../lib/format";
import type { Highlight } from "../../lib/highlight";
import type { BandShare } from "../../lib/profile";
import { providerLabel } from "../../lib/provider";
import type { SurfaceSummary } from "../../lib/surface";
import type { UnitSystem } from "../../lib/units";
import { ClimbsList } from "./ClimbsList";
import { ReprocessButton } from "./ReprocessButton";

export interface RoutePanelProps {
  route: Route;
  /**
   * The elevation profile, which sits inside this card between the figures it
   * elaborates and the gradient bar it explains.
   *
   * Handed in rather than built here, because every question the chart answers
   * — the stretch on show, the position under the pointer, the class picked out
   * of the chips — is also asked of the map, and the page is the one place both
   * views can be answered from.
   */
  profile: React.ReactNode;
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
  /** The bands this route actually has and their shares of it, gentlest first. */
  bands: BandShare[];
  highlight: Highlight | null;
  onHighlightChange: (highlight: Highlight | null) => void;
  /** The route's sustained climbs, in the order they are ridden. */
  climbs: Climb[];
  /** Opens the shared map/chart window on one climb. */
  onSelectClimb: (climb: Climb) => void;
  /**
   * How many routes the search goes back to, which the way back says.
   *
   * The count is what makes leaving a described action rather than an undo: a
   * reader who opened a route by accident is told what is behind it. Zero is a
   * library that has not arrived yet, and the control says so in words instead.
   */
  libraryCount: number;
  /** Puts the route away and gives the search pill back. */
  onClose: () => void;
  /** Each configured source's web application, keyed by provider. */
  sourceBaseUrls: Record<string, string>;
  /** The units the figures and the climbs list report distance and elevation in. */
  unitSystem: UnitSystem;
}

export function RoutePanel({
  route,
  profile,
  highestMetres,
  subtitle,
  surface,
  surfaceAbsence,
  bands,
  highlight,
  onHighlightChange,
  climbs,
  onSelectClimb,
  libraryCount,
  onClose,
  sourceBaseUrls,
  unitSystem,
}: RoutePanelProps) {
  const back =
    libraryCount > 0
      ? `Search ${libraryCount} ${libraryCount === 1 ? "route" : "routes"}`
      : "Back to search";

  return (
    <section className="panel route-panel" aria-label={route.title}>
      {/*
       * The way back, and the only thing above the name: this panel replaced
       * the search, so a reader who opened a route by accident has to be able
       * to see how to get the search back without reading the route first.
       *
       * Twice, at either end of the line. The sentence is the one that says
       * where leaving goes; the cross is the one a reader looks for without
       * reading anything, and it sits where every dismissable thing keeps it.
       */}
      <div className="route-panel__header">
        <button className="route-panel__back" type="button" onClick={onClose}>
          <IconArrowLeft size={16} stroke={2} aria-hidden="true" />
          <span>{back}</span>
        </button>
        <button
          className="route-panel__close"
          type="button"
          onClick={onClose}
          aria-label="Close the route"
        >
          <IconX size={18} stroke={2} aria-hidden="true" />
        </button>
      </div>
      <div className="route-panel__name">
        <h2 className="route-panel__title">{route.title}</h2>
        {/*
         * Which source this stage came from. A quiet label rather than a logo:
         * this is a private tool with two sources, not a marketplace, and real
         * text is what makes it distinguishable by accessible name alone.
         */}
        <span className="source-label">{providerLabel(route.provider)}</span>
        {subtitle === "" ? null : <p className="route-panel__subtitle">{subtitle}</p>}
      </div>
      <dl className="route-panel__figures">
        <div>
          <dt>Distance</dt>
          <dd>{formatDistance(route.distanceMetres, unitSystem)}</dd>
        </div>
        <div>
          <dt>Climbing</dt>
          <dd>{formatAscent(route.ascentMetres, unitSystem)}</dd>
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
          <dd>{highestMetres === null ? "—" : formatElevation(highestMetres, unitSystem)}</dd>
        </div>
      </dl>
      {profile}
      <ClimbsList climbs={climbs} onSelect={onSelectClimb} unitSystem={unitSystem} />
      {/*
       * The two mixes, as a bar and a row of chips each, and pressing a chip
       * lights that ground on both the map and the chart. They sit under the
       * plot rather than inside it: the chart folds away, and a key that
       * travelled with it would take the surface mix — which the chart does not
       * draw at all — away with it.
       */}
      <RouteKey
        surface={surface}
        surfaceAbsence={surfaceAbsence}
        bands={bands}
        highlight={highlight}
        onHighlightChange={onHighlightChange}
      />
      <div className="route-panel__actions">
        <SourceRouteLink
          provider={route.provider}
          baseUrl={sourceBaseUrls[route.provider]}
          routeId={route.routeId}
        />
        <ReprocessButton
          provider={route.provider}
          routeId={route.routeId}
          stageOrder={route.stageOrder}
        />
      </div>
    </section>
  );
}
