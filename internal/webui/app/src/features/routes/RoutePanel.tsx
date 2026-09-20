/**
 * One route, in the panel the search was in.
 *
 * Not a page. The map is the library and the library is the page, so opening a
 * route swaps what the top-left panel is holding rather than navigating away
 * from the ground the reader is looking at: the same route stays drawn, the
 * same camera moves to it, and the way back is one control on this panel.
 *
 * It answers one question — *is this the route to ride* — and nothing else.
 * Everything drawn against the route's distance is the dock's: the profile, the
 * ground in ride order, the forecast. What is left is what the route *is*, and
 * that is small enough that covering the map with it permanently is a bad
 * trade. So the panel rests as a pill and unfolds on request.
 *
 * The workspace rail has no chrome of its own, so this panel's card is the
 * only card — and `useOverlayInsets` keeps framing routes around whatever size
 * it currently is.
 *
 * Read-only over the source route. Copying seeds an unsaved local plan; nothing
 * in this panel writes back to a provider.
 */

import {
  IconCopy,
  IconLayoutNavbarCollapse,
  IconLayoutNavbarExpand,
  IconMenu2,
  IconPencil,
  IconTrendingDown,
  IconTrendingUp,
  IconX,
} from "@tabler/icons-react";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router";
import { webUIConfigQuery } from "../../api/queries";
import type { Route } from "../../api/types";
import { SourceRouteLink } from "../../components/SourceRouteLink";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../../components/ui/dropdown-menu";
import {
  formatAscent,
  formatDescent,
  formatDistance,
  formatElevation,
  formatGradient,
  formatMovingTime,
  formatMovingTimeUncertainty,
} from "../../lib/format";
import type { Highlight } from "../../lib/highlight";
import { useEffectiveAdmin } from "../../lib/identity";
import { bandEntries, surfaceEntries } from "../../lib/mix";
import type { BandShare, GradientSummary } from "../../lib/profile";
import type { SurfaceSummary } from "../../lib/surface";
import { climbingVerdict, surfaceVerdict, type VerdictTone } from "../../lib/verdict";
import type { PlannerSeed } from "../plan/planner";
import { MixRow } from "./MixRow";
import { ReprocessButton } from "./ReprocessButton";

/** A soft tinted chip in one of the page's tones: a claim about the route, not decoration. */
function Chip({ tone, children }: { tone: VerdictTone; children: React.ReactNode }) {
  const colour = tone === "info" ? "var(--accent)" : `var(--${tone})`;

  return (
    <span
      className="inline-flex shrink-0 items-center gap-0.5 rounded-full px-1.5 py-0.5 font-medium text-[11px] tabular-nums"
      style={{ color: colour, background: `color-mix(in oklab, ${colour} 12%, transparent)` }}
    >
      {children}
    </span>
  );
}

/**
 * One statement about the route: a title with a verdict beside it, and one
 * quieter line of the figures behind the statement.
 */
function Entry({
  title,
  chip,
  sub,
  value,
}: {
  title: React.ReactNode;
  chip?: React.ReactNode;
  sub?: React.ReactNode;
  value?: React.ReactNode;
}) {
  return (
    <div className="grid gap-0.5 border-[var(--rule)] border-b py-2.5 first:pt-0 last:border-b-0 last:pb-0">
      <div className="flex items-center justify-between gap-3">
        <span className="min-w-0 truncate font-medium">{title}</span>
        {chip}
      </div>
      {sub !== undefined || value !== undefined ? (
        <div className="flex items-baseline justify-between gap-3 text-[13px]">
          <span className="min-w-0 truncate text-[var(--ink-2)]">{sub}</span>
          <span className="shrink-0 font-medium tabular-nums">{value}</span>
        </div>
      ) : null}
    </div>
  );
}

function CopyAndEdit({ seed }: { seed: PlannerSeed }) {
  const navigate = useNavigate();

  return (
    <DropdownMenuItem onClick={() => navigate("/plan", { state: seed })}>
      <IconCopy aria-hidden="true" />
      Copy and edit
    </DropdownMenuItem>
  );
}

export interface RoutePanelProps {
  route: Route;
  /** An unsaved local-plan seed, available only once its source geometry arrived. */
  copySeed?: PlannerSeed | null;
  /**
   * The moving time for the stretch currently on show, in place of the whole
   * route's. Undefined restores the whole-route figure — clearing the selection,
   * or a route nothing has predicted, both read the same way here.
   */
  movingSecondsOverride?: number | undefined;
  /** The whole route's highest point, or null where there is no usable profile. */
  highestMetres: number | null;
  /** Its lowest, from the same profile and null on the same terms. */
  lowestMetres: number | null;
  /**
   * The route's gradients with up told from down.
   *
   * A gradient answers whether the ride will be hard, and only the climbing
   * decides that — so the descents are neither averaged in nor allowed to
   * stand in for the steepest climb, which is what the service's own single
   * absolute figure lets them do.
   */
  gradients: GradientSummary;
  /** Null for a route nobody has classified, which the key says in words. */
  surface: SurfaceSummary | null;
  surfaceAbsence: string;
  /** The bands this route actually has and their shares of it, gentlest first. */
  bands: BandShare[];
  highlight: Highlight | null;
  onHighlightChange: (highlight: Highlight | null) => void;
  /** Puts the highlight away on collapse, without touching the zoom. */
  onHighlightClear: () => void;
  /**
   * Whether the panel rests as a pill.
   *
   * Held by the page and sticky across routes, for the reason `ElevationProfile`
   * is collapsed the same way: a reader who put the card away did so to see more
   * map, not to see more of one route's map.
   */
  collapsed: boolean;
  onCollapsedChange: (collapsed: boolean) => void;
  /**
   * How many routes the search goes back to.
   *
   * The count is what makes leaving a described action rather than an undo: a
   * reader who opened a route by accident is told what is behind it. It is the
   * close button's accessible name, since the pill has no room for a row of its
   * own to write it on.
   */
  libraryCount: number;
  /** Puts the route away and gives the search pill back. */
  onClose: () => void;
  /** Each configured source's web application, keyed by provider. */
  sourceBaseUrls: Record<string, string>;
}

export function RoutePanel({
  route,
  copySeed,
  movingSecondsOverride,
  highestMetres,
  lowestMetres,
  gradients,
  surface,
  surfaceAbsence,
  bands,
  highlight,
  onHighlightChange,
  onHighlightClear,
  collapsed,
  onCollapsedChange,
  libraryCount,
  onClose,
  sourceBaseUrls,
}: RoutePanelProps) {
  const movingSeconds = movingSecondsOverride ?? route.movingSeconds;
  const climbing = climbingVerdict(route.ascentMetres, route.distanceMetres);
  const ground = surfaceVerdict(surface);
  // Largest share first, so the sub-line names what the route is mostly made of.
  const surfaces = surfaceEntries(surface).sort((left, right) => right.metres - left.metres);
  const effectiveAdmin = useEffectiveAdmin();
  const config = useQuery(webUIConfigQuery());

  return (
    <section
      aria-label={route.title}
      // The pill hugs its content; the card does not. Left to size itself the
      // card took its width from whichever row was widest, so a long title
      // stretched the panel and left every rule below it stopping short of
      // the edge. Open, the width is the card's and the header lives in it.
      className={`max-h-[calc(100dvh-9rem)] max-w-full overflow-y-auto rounded-xl bg-[var(--panel)] shadow-[var(--shadow)] w-[24rem]`}
    >
      {/*
       * The route's name as the panel's heading, drawn nowhere: the pill
       * below shows it, and printing it twice would spend the card's first
       * row telling a reader something they are already looking at. Without
       * it the panel has no heading at all — the document jumps from the
       * page's own h1 to the mixes' h3s, and a reader moving by heading
       * lands inside a panel about a route that never named itself.
       */}
      <h2 className="visually-hidden">{route.title}</h2>
      <div className="flex items-center gap-1.5 p-2 pl-4">
        <span className="min-w-0 max-w-[15rem] truncate font-semibold">{route.title}</span>
        {/* One pill for everything that is not the route: fold, menu, the way out. */}
        <div className="ml-auto flex shrink-0 overflow-hidden rounded-[9px] bg-[var(--muted)]">
          <button
            type="button"
            aria-expanded={!collapsed}
            aria-label={collapsed ? "Expand the route card" : "Collapse the route card"}
            onClick={() => {
              const next = !collapsed;
              onCollapsedChange(next);
              // Collapsing takes the class labels away with it, so a class
              // picked before is left with no visible cause. Clears only the
              // highlight, not a zoom the reader dragged in separately.
              if (next) {
                onHighlightClear();
              }
            }}
            className="grid h-7 w-8 place-items-center text-[var(--ink-2)] hover:bg-[var(--rule)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--accent)]"
          >
            {/* The dock's own fold glyph, for the bar at the top rather than the bottom. */}
            {collapsed ? (
              <IconLayoutNavbarExpand size={16} stroke={2} aria-hidden="true" />
            ) : (
              <IconLayoutNavbarCollapse size={16} stroke={2} aria-hidden="true" />
            )}
          </button>
          <DropdownMenu>
            <DropdownMenuTrigger
              aria-label="More about this route"
              className="grid h-7 w-8 place-items-center text-[var(--ink-2)] hover:bg-[var(--rule)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--accent)] border-[var(--rule)] border-l"
            >
              <IconMenu2 size={16} stroke={2} aria-hidden="true" />
            </DropdownMenuTrigger>
            {/*
             * `w-auto` because the menu's own width follows its anchor, and the
             * anchor here is a 28-pixel icon button.
             */}
            <DropdownMenuContent align="end" className="w-auto min-w-52">
              {/*
               * The two quiet actions that used to hold a bordered row of their
               * own at the foot of the card. Both are rare — one leaves for the
               * provider, the other asks the service to work the route out
               * again — and a row spent on them is a row not spent on the route.
               */}
              <SourceRouteLink
                provider={route.provider}
                baseUrl={sourceBaseUrls[route.provider]}
                sourceRouteId={route.sourceRouteId}
              />
              {effectiveAdmin && config.data?.planning && route.provider === "local" ? (
                <DropdownMenuItem render={<Link to={`/plan/${route.sourceRouteId}`} />}>
                  <IconPencil aria-hidden="true" />
                  Edit
                </DropdownMenuItem>
              ) : null}
              {effectiveAdmin && config.data?.planning && route.provider !== "local" && copySeed ? (
                <CopyAndEdit seed={copySeed} />
              ) : null}
              {effectiveAdmin ? (
                <>
                  <DropdownMenuSeparator />
                  <ReprocessButton
                    provider={route.provider}
                    sourceRouteId={route.sourceRouteId}
                    stageOrder={route.stageOrder}
                  />
                </>
              ) : null}
            </DropdownMenuContent>
          </DropdownMenu>
          <button
            type="button"
            onClick={onClose}
            aria-label={
              // Zero is the listing still loading, not an empty library, and
              // "go back to 0 routes" reads as the second.
              libraryCount === 0
                ? "Close the route and go back to the library"
                : `Close the route and go back to ${libraryCount} ${libraryCount === 1 ? "route" : "routes"}`
            }
            className="grid h-7 w-8 place-items-center text-[var(--ink-2)] hover:bg-[var(--rule)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--accent)] border-[var(--rule)] border-l"
          >
            <IconX size={16} stroke={2} aria-hidden="true" />
          </button>
        </div>
      </div>
      {/*
       * Folded, the figures a ride is decided on stay on the second line,
       * with the two verdicts: open, they are the first entries below.
       */}
      {collapsed ? (
        <div className="flex items-center justify-between gap-3 px-4 pb-2.5 text-sm">
          <span className="text-[var(--ink-2)] tabular-nums">
            {formatDistance(route.distanceMetres)} · {formatAscent(route.ascentMetres)}
          </span>
          <span className="inline-flex gap-1">
            <Chip tone="info">
              <span className="sr-only">Moving time </span>
              {formatMovingTime(movingSeconds)}
            </Chip>
            {climbing ? <Chip tone={climbing.tone}>{climbing.label}</Chip> : null}
          </span>
        </div>
      ) : (
        <div className="grid w-full gap-3 px-3 pt-2 pb-3">
          <div>
            {/*
             * Predicted, not measured: the chip says "moving time", carries
             * no stops or weather, and the sub-line names how far off that
             * estimate usually runs, from the frozen profile's own benchmark.
             */}
            <Entry
              title={formatDistance(route.distanceMetres)}
              chip={
                <Chip tone="info">
                  <span className="sr-only">Moving time </span>
                  {formatMovingTime(movingSeconds)}
                </Chip>
              }
              sub={
                movingSeconds === undefined
                  ? "no moving time predicted"
                  : `moving time ${formatMovingTimeUncertainty(route.validation) ?? "predicted"}`
              }
              value={
                lowestMetres === null || highestMetres === null
                  ? undefined
                  : `${Math.round(lowestMetres).toLocaleString()}–${formatElevation(highestMetres)}`
              }
            />
            <Entry
              title={`${formatAscent(route.ascentMetres)} of climbing`}
              chip={climbing ? <Chip tone={climbing.tone}>{climbing.label}</Chip> : undefined}
              sub={`${formatGradient(gradients.averageClimbing)} average · ${formatDescent(route.descentMetres)} down`}
              value={
                <span className="inline-flex items-center gap-1">
                  <span className="font-normal text-[var(--ink-2)]">max</span>
                  <Chip tone="alert">
                    <IconTrendingUp size={12} stroke={2} aria-hidden="true" />
                    <span className="sr-only">climb</span>
                    {formatGradient(gradients.steepestClimbing)}
                  </Chip>
                  <Chip tone="hold">
                    <IconTrendingDown size={12} stroke={2} aria-hidden="true" />
                    <span className="sr-only">descent</span>
                    {formatGradient(gradients.steepestDescent)}
                  </Chip>
                </span>
              }
            />
            <Entry
              title={ground?.title ?? "Surface"}
              chip={ground ? <Chip tone={ground.tone}>{ground.label}</Chip> : undefined}
              sub={
                ground
                  ? surfaces
                      .slice(0, 2)
                      .map(
                        (entry) => `${entry.label.toLowerCase()} ${formatDistance(entry.metres)}`,
                      )
                      .join(" · ")
                  : surfaceAbsence
              }
              value={ground ? `unsealed ${formatDistance(ground.unsealedMetres)}` : undefined}
            />
          </div>
          {/*
           * Mirrored: gradient's tags above its bar, surface's below its
           * own, so the two meet with nothing between them. What a reader is
           * comparing — how much of the route is steep against how much of
           * it is loose — sits a couple of pixels apart rather than a
           * heading apart.
           */}
          <div className="grid gap-0.5">
            <MixRow
              classesLabel="Gradient bands"
              entries={bandEntries(bands, route.distanceMetres)}
              absence="No elevation data."
              tagSide="above"
              highlight={highlight}
              onHighlightChange={onHighlightChange}
            />
            <MixRow
              classesLabel="Surface classes"
              entries={surfaces}
              absence={surfaceAbsence}
              tagSide="below"
              highlight={highlight}
              onHighlightChange={onHighlightChange}
            />
          </div>
        </div>
      )}
    </section>
  );
}
