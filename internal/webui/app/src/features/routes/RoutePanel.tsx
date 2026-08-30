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
 * The pill is the mechanism `SearchPanel` already uses: `data-compact-workspace`
 * makes the shell drop its own background, padding, shadow and ring, so a panel
 * that brings its own chrome gets a floating pill for free — and
 * `useOverlayInsets` keeps framing routes around whatever size it currently is.
 *
 * Read-only by design. There are deliberately no editing affordances, and the
 * service writes nothing back to VeloPlanner — the two quiet actions in the
 * overflow either leave for the provider or ask this service to work the route
 * out again.
 */

import {
  IconChevronsRight,
  IconDots,
  IconTrendingDown,
  IconTrendingUp,
  IconX,
} from "@tabler/icons-react";
import type { Route } from "../../api/types";
import { SourceRouteLink } from "../../components/SourceRouteLink";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../../components/ui/dropdown-menu";
import { Separator } from "../../components/ui/separator";
import {
  formatAscent,
  formatDistance,
  formatElevation,
  formatGradient,
  formatMovingTime,
  formatMovingTimeUncertainty,
} from "../../lib/format";
import type { Highlight } from "../../lib/highlight";
import { bandEntries, surfaceEntries } from "../../lib/mix";
import type { BandShare, GradientSummary } from "../../lib/profile";
import type { SurfaceSummary } from "../../lib/surface";
import { elevationValue, type UnitSystem } from "../../lib/units";
import { MixSection } from "./MixSection";
import { ReprocessButton } from "./ReprocessButton";

/**
 * One figure, as a line rather than as a stack.
 *
 * The name left and the number right, so the numbers land on two rules down
 * the card and a reader comparing them reads down a column rather than
 * hunting. Stacked, each pair cost two lines and the figures sat wherever
 * their labels happened to end.
 *
 * `items-baseline` because the two really are different sizes on one line,
 * which is what a baseline is for — unlike a row taller than its own text,
 * where it only pins the words to the ceiling.
 */
function Figure({
  term,
  span,
  children,
}: {
  term: React.ReactNode;
  /** Both columns, for a figure with no partner to sit beside. */
  span?: boolean;
  children: React.ReactNode;
}) {
  return (
    <div className={`flex items-baseline justify-between gap-2 ${span ? "col-span-2" : ""}`}>
      <dt className="flex min-w-0 items-center gap-1 truncate text-[11px] text-[var(--ink-2)] [&_svg]:size-3 [&_svg]:shrink-0">
        {term}
      </dt>
      <dd className="shrink-0 text-sm leading-tight tabular-nums">{children}</dd>
    </div>
  );
}

export interface RoutePanelProps {
  route: Route;
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
  unitSystem: UnitSystem;
}

export function RoutePanel({
  route,
  movingSecondsOverride,
  highestMetres,
  lowestMetres,
  gradients,
  surface,
  surfaceAbsence,
  bands,
  highlight,
  onHighlightChange,
  collapsed,
  onCollapsedChange,
  libraryCount,
  onClose,
  sourceBaseUrls,
  unitSystem,
}: RoutePanelProps) {
  const movingSeconds = movingSecondsOverride ?? route.movingSeconds;

  return (
    // The shell strips its own card off whatever carries this, which is what
    // lets the pill be a pill rather than a pill inside a panel.
    <div data-compact-workspace="" className="w-fit max-w-full">
      <section
        aria-label={route.title}
        // The pill hugs its content; the card does not. Left to size itself the
        // card took its width from whichever row was widest, so a long title
        // stretched the panel and left every rule below it stopping short of
        // the edge. Open, the width is the card's and the header lives in it.
        className={`max-h-[calc(100dvh-9rem)] max-w-full overflow-y-auto rounded-xl bg-[var(--panel)] shadow-[var(--shadow)] ring-1 ring-black/5 ${collapsed ? "w-fit" : "w-[23rem]"}`}
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
        <div className="flex items-center gap-1 p-1.5">
          <button
            type="button"
            aria-expanded={!collapsed}
            onClick={() => {
              const next = !collapsed;
              onCollapsedChange(next);
              // Collapsing takes the class labels away with it, and a pressed
              // one is the only way to give the whole route back. Rather than
              // leave the map lit with no visible cause, putting the card away
              // puts the question away too.
              if (next) {
                onHighlightChange(null);
              }
            }}
            className="flex min-w-0 items-center gap-2 rounded-lg px-2 py-1 text-left hover:bg-[var(--base)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--accent)]"
          >
            <IconChevronsRight
              size={16}
              stroke={2}
              aria-hidden="true"
              className={collapsed ? "transition-transform" : "rotate-90 transition-transform"}
            />
            <span className="min-w-0 max-w-[15rem] truncate font-semibold">{route.title}</span>
            {/*
             * The two figures a ride is decided on, on the line that is visible
             * far more often than the card is. A pill that only named the route
             * would make every reading of them cost a press.
             *
             * Only on the pill: open, they are the first two rows of the list
             * immediately below, and the width they cost is the width the title
             * then has to truncate into.
             */}
            {collapsed ? (
              <span className="shrink-0 text-sm text-[var(--ink-2)] tabular-nums">
                {formatDistance(route.distanceMetres, unitSystem)} ·{" "}
                {formatAscent(route.ascentMetres, unitSystem)}
              </span>
            ) : null}
          </button>
          <DropdownMenu>
            <DropdownMenuTrigger
              aria-label="More about this route"
              className="ml-auto rounded-lg p-1.5 text-[var(--ink-2)] hover:bg-[var(--base)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--accent)]"
            >
              <IconDots size={16} stroke={2} aria-hidden="true" />
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
              <DropdownMenuSeparator />
              <ReprocessButton
                provider={route.provider}
                sourceRouteId={route.sourceRouteId}
                stageOrder={route.stageOrder}
              />
            </DropdownMenuContent>
          </DropdownMenu>
          <button
            type="button"
            onClick={onClose}
            aria-label={`Close the route and go back to ${libraryCount} ${libraryCount === 1 ? "route" : "routes"}`}
            className="rounded-lg p-1.5 text-[var(--ink-2)] hover:bg-[var(--base)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--accent)]"
          >
            <IconX size={16} stroke={2} aria-hidden="true" />
          </button>
        </div>
        {collapsed ? null : (
          <div className="grid w-full gap-2 px-3 pt-2 pb-3">
            {/*
             * Base UI's separator always carries `role="separator"`, with no
             * decorative escape hatch. These three rules only group one card's
             * parts, so the role says a little more than they mean — the cost
             * of dividing sections with the component rather than a border.
             *
             * Full-bleed against the card's own padding, so the rule reads as
             * the card's division rather than as a line inside its content,
             * and `data-horizontal:w-auto` to beat the component's own
             * `w-full`, which would measure the padding box and fall short.
             */}
            <Separator className="-mx-3 -mt-2 data-horizontal:w-auto" />
            {/*
             * Paired by row: what it is and how much it climbs, then the two
             * gradients, then the two ends of its elevation. The grid means
             * something that way — a reader comparing the pair reads across —
             * where before it was seven figures reflowed into two columns.
             */}
            <dl className="grid grid-cols-2 gap-x-4 gap-y-1">
              <Figure term="Distance">{formatDistance(route.distanceMetres, unitSystem)}</Figure>
              <Figure term="Ascent">{formatAscent(route.ascentMetres, unitSystem)}</Figure>
              {/*
               * One figure rather than two: the two ends of a range are read
               * together or not at all, and asking for the height of the top
               * without the height of the bottom is asking half a question.
               * It is how the dock has always said it.
               */}
              <Figure term="Elevation">
                {lowestMetres === null || highestMetres === null
                  ? "—"
                  : // The unit once, on the end it belongs to. Printed on both
                    // ends the pair is the widest figure on the card, and in
                    // feet — where both ends are four digits — it is wide
                    // enough to start eating its own label.
                    `${Math.round(elevationValue(lowestMetres, unitSystem)).toLocaleString()}–${formatElevation(highestMetres, unitSystem)}`}
              </Figure>
              <Figure term="Avg climbing">{formatGradient(gradients.averageClimbing)}</Figure>
              {/*
               * The steepest each way, which the service's own figure cannot
               * say: it takes the absolute value, so a savage descent reaches
               * the page as a savage climb. The glyph carries the direction and
               * the word carries the rest, because two rows reading "Max" and
               * differing only by an arrow would rest the whole distinction on
               * a twelve-pixel picture.
               */}
              <Figure
                term={
                  <>
                    <IconTrendingUp stroke={2} aria-hidden="true" />
                    Max climb
                  </>
                }
              >
                {formatGradient(gradients.steepestClimbing)}
              </Figure>
              <Figure
                term={
                  <>
                    <IconTrendingDown stroke={2} aria-hidden="true" />
                    Max descent
                  </>
                }
              >
                {formatGradient(gradients.steepestDescent)}
              </Figure>
              {/*
               * Predicted, not measured — the label says "moving time", not
               * "arrival time", and carries no stops, traffic or day-specific
               * weather. The qualifier names how far off that estimate usually
               * runs, from the frozen profile's own held-out benchmark.
               */}
              <Figure term="Moving time" span>
                {formatMovingTime(movingSeconds)}
                {movingSeconds !== undefined && route.validation ? (
                  <span className="ml-1 text-[11px] font-normal text-[var(--ink-2)]">
                    {formatMovingTimeUncertainty(route.validation)}
                  </span>
                ) : null}
              </Figure>
            </dl>
            <MixSection
              bands={bandEntries(bands, route.distanceMetres)}
              surface={surfaceEntries(surface)}
              surfaceAbsence={surfaceAbsence}
              highlight={highlight}
              onHighlightChange={onHighlightChange}
              unitSystem={unitSystem}
            />
          </div>
        )}
      </section>
    </div>
  );
}
