/**
 * One route, as a row of two answers rather than a run of figures.
 *
 * **The ride** — how far, how long, and over what ground. **The climbing** —
 * how much of it, how steep it ever gets, and in what mix. Each cell leads with
 * its pair of figures and divides underneath them, so the two have the same
 * shape and can be read against each other.
 *
 * Both divisions come off the geometry the page already fetches for the glyphs,
 * so neither costs a request. Neither is on the listing: the surface mix was
 * previously only a thing to filter by, and the gradient mix could not be seen
 * at all without opening the route on the atlas.
 *
 * This is the wide form. Below the one breakpoint the page keeps its stacked
 * card, which still reads the plain figures — see `CataloguePage`.
 */

import { Link } from "react-router";
import type { Position, Route, SurfaceRange } from "../../api/types";
import { RouteGlyph } from "../../components/route/RouteGlyph";
import { formatAscent, formatDistance, formatGradient, formatMovingTime } from "../../lib/format";
import { GRADIENT_BANDS, gradientBand, gradientShares } from "../../lib/profile";
import type { RouteChange } from "../../lib/seenRoutes";
import { SURFACE_STYLES, summariseSurface } from "../../lib/surface";
import type { UnitSystem } from "../../lib/units";
import { SharePill } from "./SharePill";

/** The custom property each surface class is painted from. */
const SURFACE_VARIABLE: Record<keyof typeof SURFACE_STYLES, string> = {
  asphalt: "--surface-asphalt",
  paving: "--surface-paving",
  compacted: "--surface-compacted",
  gravel: "--surface-gravel",
  ground: "--surface-ground",
  unknown: "--surface-unsurveyed",
};

/**
 * Whether this route has moved since the reader last opened it, as the row's
 * own left border.
 *
 * The states differ by line style — new is solid, updated is dashed — rather
 * than by colour, and the word itself stays in the row as text only a screen
 * reader reads. A border cannot carry a name, so the name lives beside it.
 *
 * The dash is set on the left edge alone. `border-dashed` is a shorthand for
 * every edge, and the row draws a top rule of its own: an updated route took
 * the dashes across that rule too, which reads as a broken table rather than
 * as a marked row.
 */
const CHANGE_BORDER =
  "border-l-[3px] border-l-transparent data-[change=new]:border-l-[var(--accent)] data-[change=updated]:border-l-[var(--accent)] data-[change=updated]:[border-left-style:dashed]";

/** The source route this one came off, where the title does not already say it. */
function secondName(route: Route): string | null {
  return route.sourceRouteName !== route.title ? route.sourceRouteName : null;
}

export interface CatalogueRowProps {
  route: Route;
  /** The route's shape, or empty until its geometry arrives. */
  coordinates: Position[];
  /** Its classified ground, or undefined where nothing has classified it. */
  surface: SurfaceRange[] | undefined;
  change: RouteChange;
  unitSystem: UnitSystem;
  /** The atlas address this route opens at. */
  to: string;
}

export function CatalogueRow({
  route,
  coordinates,
  surface,
  change,
  unitSystem,
  to,
}: CatalogueRowProps) {
  const where = secondName(route);
  /*
   * Whether this route's geometry is in hand at all. Both divisions below come
   * off it, and both are empty before it arrives — so without this the page
   * would answer "surface not classified" and "no elevation data" for every
   * route in the moment before its geometry lands, stating a result it does
   * not have yet. Nothing is said until there is something to say.
   */
  const measured = coordinates.length > 0;
  const summary = surface ? summariseSurface(coordinates, surface) : null;
  const bands = gradientShares(coordinates);

  return (
    <tr
      className={`border-[var(--rule)] border-t align-top hover:bg-[var(--base)] ${CHANGE_BORDER}`}
      data-change={change ?? undefined}
    >
      <td className="py-3 pl-3">
        {change === null ? null : (
          <span className="sr-only">{change === "new" ? "New" : "Updated"}</span>
        )}
        <span className="block size-9">
          <RouteGlyph
            coordinates={coordinates}
            title={route.title}
            band={gradientBand(route.maxGradientPercent)}
          />
        </span>
      </td>
      <td className="px-3 py-3">
        <Link
          to={to}
          className="block font-semibold hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--accent)]"
        >
          {route.title}
        </Link>
        {where === null ? null : <span className="block text-xs text-[var(--ink-2)]">{where}</span>}
      </td>
      <td className="w-80 px-3 py-3">
        <span className="block text-right tabular-nums">
          {formatDistance(route.distanceMetres, unitSystem)}
          {route.movingSeconds === undefined ? null : (
            <span className="ml-2 text-[var(--ink-2)]">
              {formatMovingTime(route.movingSeconds)}
            </span>
          )}
        </span>
        {summary === null ? (
          !measured ? null : (
            <span className="mt-1.5 block text-right text-[11px] text-[var(--ink-2)]">
              surface not classified
            </span>
          )
        ) : (
          <span className="mt-1.5 flex flex-wrap justify-end gap-1">
            {summary.shares.map((entry) => (
              <SharePill
                key={entry.kind}
                colour={`var(${SURFACE_VARIABLE[entry.kind]})`}
                label={SURFACE_STYLES[entry.kind].label}
                share={entry.share}
              />
            ))}
          </span>
        )}
      </td>
      <td className="w-80 px-3 py-3">
        <span className="block text-right tabular-nums">
          {bands.length === 0 ? (
            <span className="text-[var(--ink-2)]">{measured ? "no elevation data" : ""}</span>
          ) : (
            <>
              {formatAscent(route.ascentMetres, unitSystem)}
              <span className="ml-2 text-[var(--ink-2)]">
                max {formatGradient(route.maxGradientPercent)}
              </span>
            </>
          )}
        </span>
        {bands.length === 0 ? null : (
          <span className="mt-1.5 flex flex-wrap justify-end gap-1">
            {bands.map((entry) => (
              <SharePill
                key={entry.band}
                colour={`var(--grade-${entry.band})`}
                label={GRADIENT_BANDS[entry.band]?.label ?? ""}
                share={entry.share}
              />
            ))}
          </span>
        )}
      </td>
    </tr>
  );
}
