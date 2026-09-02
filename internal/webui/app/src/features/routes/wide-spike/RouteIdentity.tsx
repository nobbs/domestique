/**
 * What the route *is*, with nothing drawn.
 *
 * The composition spike's one shared piece. Every alternative there is an
 * answer to the same complaint: with the wide sheet open, steepness and ground
 * are on screen twice — as the card's two upright bars, and again as the
 * chart's own banding and the sheet's ribbon. Two encodings of one fact, six
 * hundred pixels apart, in a panel whose whole justification was that it does
 * not repeat the thing above it.
 *
 * So the drawn readings become the sheet's alone, and what is left of the
 * route is this: its name, where it came from, its figures, and the one line
 * about its climbs. Laid down a column or along a row, because where it goes
 * is exactly what the alternatives disagree about.
 */

import type { Route } from "../../../api/types";
import type { Climb } from "../../../lib/climbs";
import {
  formatAscent,
  formatDistance,
  formatElevation,
  formatGradient,
  formatMovingTime,
  formatMovingTimeUncertainty,
} from "../../../lib/format";
import { providerLabel } from "../../../lib/provider";
import type { UnitSystem } from "../../../lib/units";
import { climbSentence } from "../panel-spike/shared";

function Figure({ term, children }: { term: string; children: React.ReactNode }) {
  return (
    <div>
      <dt className="text-[11px] leading-none text-[var(--ink-2)]">{term}</dt>
      <dd className="text-sm leading-tight tabular-nums">{children}</dd>
    </div>
  );
}

/** Who the route is: its name, where it came from, and when it was read. */
export function RouteHeading({
  route,
  subtitle,
  named = false,
}: {
  route: Route;
  subtitle: string;
  /** Off where a pill above is already carrying the name. */
  named?: boolean;
}) {
  return (
    <div className="min-w-0">
      {!named ? null : (
        <h2 className="truncate text-base leading-tight font-semibold tracking-tight">
          {route.title}
        </h2>
      )}
      <p className="text-xs text-[var(--ink-2)]">
        <span className="font-semibold tracking-[0.06em] uppercase">
          {providerLabel(route.provider)}
        </span>
        {subtitle === "" ? null : ` · ${subtitle}`}
      </p>
    </div>
  );
}

/** What the route measures, as a block that can stand on its own. */
export function RouteFigures({
  route,
  movingSeconds,
  highestMetres,
  unitSystem,
  layout = "column",
  columns = 2,
  className,
}: {
  route: Route;
  movingSeconds: number | undefined;
  highestMetres: number | null;
  unitSystem: UnitSystem;
  layout?: "column" | "row";
  columns?: 2 | 3;
  className?: string;
}) {
  return (
    <dl
      className={`${
        layout === "row"
          ? "flex shrink-0 items-baseline gap-5"
          : `grid gap-x-3 gap-y-1.5 ${columns === 3 ? "grid-cols-3" : "grid-cols-2"}`
      } ${className ?? ""}`}
    >
      <Figure term="Distance">{formatDistance(route.distanceMetres, unitSystem)}</Figure>
      <Figure term="Ascent">{formatAscent(route.ascentMetres, unitSystem)}</Figure>
      <Figure term="Max gradient">{formatGradient(route.maxGradientPercent)}</Figure>
      <Figure term="Highest">
        {highestMetres === null ? "—" : formatElevation(highestMetres, unitSystem)}
      </Figure>
      <div className={layout === "row" || columns === 3 ? undefined : "col-span-2"}>
        <dt className="text-[11px] leading-none text-[var(--ink-2)]">Moving time</dt>
        <dd className="text-sm leading-tight tabular-nums">
          {formatMovingTime(movingSeconds)}
          {movingSeconds !== undefined && route.validation ? (
            <span className="ml-1 text-[11px] text-[var(--ink-2)]">
              {formatMovingTimeUncertainty(route.validation)}
            </span>
          ) : null}
        </dd>
      </div>
    </dl>
  );
}

/** The two together, for an arrangement that wants them stacked. */
export function RouteIdentity({
  route,
  subtitle,
  movingSeconds,
  highestMetres,
  climbs,
  unitSystem,
  layout = "column",
  named = false,
  climbLine = true,
  columns = 2,
}: {
  route: Route;
  subtitle: string;
  movingSeconds: number | undefined;
  highestMetres: number | null;
  climbs: Climb[];
  unitSystem: UnitSystem;
  layout?: "column" | "row";
  named?: boolean;
  climbLine?: boolean;
  columns?: 2 | 3;
}) {
  const summary = climbLine ? climbSentence(climbs, unitSystem) : null;

  return (
    <div className={layout === "row" ? "flex min-w-0 items-baseline gap-5" : "grid min-w-0 gap-2"}>
      <RouteHeading route={route} subtitle={subtitle} named={named} />
      <RouteFigures
        route={route}
        movingSeconds={movingSeconds}
        highestMetres={highestMetres}
        unitSystem={unitSystem}
        layout={layout}
        columns={columns}
      />
      {summary === null ? null : (
        <p
          className={
            layout === "row"
              ? "shrink-0 text-xs text-[var(--ink-2)]"
              : "border-t border-[var(--rule)] pt-2 text-xs text-[var(--ink-2)]"
          }
        >
          {summary}
        </p>
      )}
    </div>
  );
}
