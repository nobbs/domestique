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
  /** Down a card, or along the head of a dock. */
  layout?: "column" | "row";
  /** Whether the route's name is here, or on a pill above that already has it. */
  named?: boolean;
  /**
   * Whether the climbs summary is drawn here.
   *
   * Off where a foldable climbs section follows, since that section's own
   * trigger *is* this line — printing it twice would put a summary directly
   * above the control that repeats it.
   */
  climbLine?: boolean;
  /**
   * How many figures share a row in the column layout.
   *
   * Three where the card also carries the mixes and a climbs list: five
   * figures over two rows instead of three is a row of height back, and the
   * card is competing with the dock for the same screen.
   */
  columns?: 2 | 3;
}) {
  const summary = climbLine ? climbSentence(climbs, unitSystem) : null;

  return (
    // `min-w-0` on both: a grid or flex item defaults to its content's minimum
    // width, so without it the title refuses to truncate and shoulders its way
    // out of whatever column it was given.
    <div className={layout === "row" ? "flex min-w-0 items-baseline gap-5" : "grid min-w-0 gap-2"}>
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
      <dl
        className={
          layout === "row"
            ? "flex shrink-0 items-baseline gap-5"
            : `grid gap-x-3 gap-y-1.5 ${columns === 3 ? "grid-cols-3" : "grid-cols-2"}`
        }
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
