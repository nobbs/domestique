/**
 * The two mixes a route is described by, as rows and as segments.
 *
 * One place for what a steepness band or a class of ground is *called* and what
 * it is *painted in*, so the several things that draw them — an upright bar in
 * the route panel, a ribbon along the dock's axis, a chart's own banding —
 * cannot drift into disagreeing about either.
 *
 * The properties rather than `bandColour`/`surfaceColour`, which take the
 * cartography's own darkness because they paint marks *on the map*. Nothing
 * here is on the map, so these follow the page's theme like every other colour
 * in a panel.
 */

import type { SurfaceKind } from "../api/types";
import type { Highlight } from "./highlight";
import type { BandShare } from "./profile";
import { GRADIENT_BANDS } from "./profile";
import type { SurfaceSummary } from "./surface";
import { SURFACE_STYLES } from "./surface";

export function bandVariable(band: number): string {
  return `var(--grade-${band})`;
}

const SURFACE_VARIABLE: Record<SurfaceKind, string> = {
  asphalt: "--surface-asphalt",
  paving: "--surface-paving",
  compacted: "--surface-compacted",
  gravel: "--surface-gravel",
  ground: "--surface-ground",
  unknown: "--surface-unsurveyed",
};

export function surfaceVariable(kind: SurfaceKind): string {
  return `var(${SURFACE_VARIABLE[kind]})`;
}

export function bandLabel(band: number): string {
  return GRADIENT_BANDS[band]?.label ?? "";
}

export function surfaceLabel(kind: SurfaceKind): string {
  return SURFACE_STYLES[kind].label;
}

/** `0.084` as `8%`, and anything that would round to nothing as `<1%`. */
export function formatShare(share: number): string {
  const percent = share * 100;

  return percent < 0.5 && percent > 0 ? "<1%" : `${Math.round(percent)}%`;
}

/** One class of one mix, as the panels draw it. */
export interface MixEntry {
  highlight: Highlight;
  label: string;
  description: string;
  share: number;
  /** The same quantity as ground rather than as a proportion. */
  metres: number;
  colour: string;
}

/**
 * The steepness bands as rows.
 *
 * The route's own length is needed because a band knows only its share of the
 * route. It is the figure the panel prints everywhere else, so a caller that
 * multiplied by the geometry's measured length instead could report totals that
 * do not add up to the distance beside them.
 */
export function bandEntries(bands: BandShare[], totalMetres: number): MixEntry[] {
  return bands.map((entry) => ({
    highlight: { type: "band", band: entry.band },
    label: bandLabel(entry.band),
    description: GRADIENT_BANDS[entry.band]?.description ?? "",
    share: entry.share,
    metres: entry.share * totalMetres,
    colour: bandVariable(entry.band),
  }));
}

export function surfaceEntries(surface: SurfaceSummary | null): MixEntry[] {
  return (surface?.shares ?? []).map((entry) => ({
    highlight: { type: "surface", kind: entry.kind },
    label: surfaceLabel(entry.kind),
    description: SURFACE_STYLES[entry.kind].description,
    share: entry.share,
    // Measured by the classifier rather than derived from the share, which is
    // the same number one rounding earlier.
    metres: entry.metres,
    colour: surfaceVariable(entry.kind),
  }));
}

/** One class's stretch of the route, as a ribbon draws it. */
export interface Segment {
  key: string;
  colour: string;
  /** Of the whole route, from 0 to 1. */
  share: number;
  highlight: Highlight;
}

/** The steepness runs and the ground bands, in the order they are ridden. */
export function gradientSegments(runs: BandShare[]): Segment[] {
  return runs.map((run, index) => ({
    key: `${index}`,
    colour: bandVariable(run.band),
    share: run.share,
    highlight: { type: "band", band: run.band },
  }));
}

export function groundSegments(surface: SurfaceSummary | null): Segment[] {
  if (surface === null || surface.totalMetres <= 0) {
    return [];
  }

  return surface.bands.map((band, index) => ({
    key: `${index}`,
    colour: surfaceVariable(band.kind),
    share: (band.endMetres - band.startMetres) / surface.totalMetres,
    highlight: { type: "surface", kind: band.kind },
  }));
}
