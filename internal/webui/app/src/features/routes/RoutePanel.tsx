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
import { Button } from "../../components/Button";
import { RouteLegend } from "../../components/route/RouteLegend";
import { SourceRouteLink } from "../../components/SourceRouteLink";
import type { Climb } from "../../lib/climbs";
import {
  formatAscent,
  formatDistance,
  formatElevation,
  formatGradient,
  formatMovingTime,
  formatMovingTimeUncertainty,
} from "../../lib/format";
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
   * The moving time for the elevation-profile stretch currently on show, in
   * place of the whole stage's. Undefined restores the whole-stage figure —
   * clearing the selection, or a stage nothing has predicted, both read the
   * same way here.
   */
  movingSecondsOverride?: number | undefined;
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
  movingSecondsOverride,
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
  const movingSeconds = movingSecondsOverride ?? route.movingSeconds;

  return (
    <section className="flex w-[32.5rem] max-w-full flex-col gap-4" aria-label={route.title}>
      {/*
       * The way back, and the only thing above the name: this panel replaced
       * the search, so a reader who opened a route by accident has to be able
       * to see how to get the search back without reading the route first.
       *
       * Twice, at either end of the line. The sentence is the one that says
       * where leaving goes; the cross is the one a reader looks for without
       * reading anything, and it sits where every dismissable thing keeps it.
       */}
      <div className="flex items-center justify-between gap-2">
        <Button variant="ghost" icon={<IconArrowLeft stroke={2} />} onClick={onClose}>
          {back}
        </Button>
        <Button
          variant="ghost"
          icon={<IconX stroke={2} />}
          onClick={onClose}
          aria-label="Close the route"
        />
      </div>
      <div>
        <h2 className="text-xl font-semibold tracking-tight">{route.title}</h2>
        {/*
         * Which source this stage came from. A quiet label rather than a logo:
         * this is a private tool with two sources, not a marketplace, and real
         * text is what makes it distinguishable by accessible name alone.
         */}
        <span className="text-xs font-semibold tracking-[0.06em] text-[var(--ink-2)] uppercase">
          {providerLabel(route.provider)}
        </span>
        {subtitle === "" ? null : <p className="mt-1 text-sm text-[var(--ink-2)]">{subtitle}</p>}
      </div>
      <dl className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm sm:grid-cols-3">
        <div>
          <dt>Distance</dt>
          <dd>{formatDistance(route.distanceMetres, unitSystem)}</dd>
        </div>
        <div>
          <dt>Ascent</dt>
          <dd>{formatAscent(route.ascentMetres, unitSystem)}</dd>
        </div>
        <div>
          <dt>Max gradient</dt>
          <dd>{formatGradient(route.maxGradientPercent)}</dd>
        </div>
        <div>
          <dt>Moving time</dt>
          {/*
           * Predicted, not measured — the label says "moving time", not
           * "arrival time", and carries no stops, traffic, or day-specific
           * weather. The qualifier names how far off that estimate usually
           * runs, from the frozen profile's own held-out benchmark, and is
           * absent whenever the loaded profile carries no measured result.
           */}
          <dd>
            {formatMovingTime(movingSeconds)}
            {movingSeconds !== undefined && route.validation ? (
              <span className="ml-1 text-xs text-[var(--ink-2)]">
                {formatMovingTimeUncertainty(route.validation)}
              </span>
            ) : null}
          </dd>
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
      <RouteLegend
        surface={surface}
        surfaceAbsence={surfaceAbsence}
        bands={bands}
        highlight={highlight}
        onHighlightChange={onHighlightChange}
      />
      <div className="flex flex-wrap gap-2 border-t border-[var(--rule)] pt-4">
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
