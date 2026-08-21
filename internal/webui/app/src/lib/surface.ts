/**
 * Presentation of a stage's surface classification.
 *
 * The service reports which stretches of a route are sealed, firm, loose, or
 * soft. This module decides how that reads: the colour each class wears, and
 * the measurements the map, the key, and the elevation readout all take from
 * one place so they cannot disagree.
 *
 * Lengths are measured here rather than taken from the API because a share needs
 * metres per class, and the API reports one total. They are measured with the
 * elevation profile's own ruler, so the figures agree with the distance shown
 * beside them and the strip sits under the climb it describes.
 */

import type { Position, SurfaceKind, SurfaceRange } from "../api/types";
import { SURFACE_KINDS } from "../api/types";
import type { CoordinateRange } from "./profile";
import { cumulativeMetres } from "./profile";
import { splitAtRanges } from "./routeLines";

/** How one class is named, explained, and painted. */
interface SurfaceStyle {
  label: string;
  /** What the class means, for a legend that has to explain "compacted". */
  description: string;
  /** The class's colour in each theme. See `SurfaceColours`. */
  colour: SurfaceColours;
}

/**
 * A colour that has to work over both basemaps.
 *
 * MapLibre paints on a canvas and cannot resolve a custom property, and the
 * elevation strip is drawn from the same values so the two cannot disagree — so
 * the pair is carried here rather than left to CSS. The same six pairs are
 * declared as `--surface-*` in index.css for the swatches in the key, which are
 * ordinary DOM. Both copies must stay in step.
 */
interface SurfaceColours {
  light: string;
  dark: string;
}

/**
 * The width the classified route is drawn at, in pixels.
 */
export const SURFACE_LINE_WIDTH = 4;

/**
 * How each class is drawn.
 *
 * Colour is the only channel. The dash patterns that used to run alongside it
 * are gone: they were a second reading of the same fact that cost the map its
 * legibility at every zoom where a dash was shorter than a bend, and this
 * service has one operator, who has said which channel they want. The classes
 * are ordered smooth to rough and the six hues are spread as far apart as six
 * allow, separated in lightness as well so gravel, ground and paving do not read
 * as neighbours.
 *
 * Unsurveyed is deliberately the quietest thing on the map. It says the route
 * goes here and nobody has recorded what it is made of, which is all that is
 * actually known.
 */
export const SURFACE_STYLES: Record<SurfaceKind, SurfaceStyle> = {
  asphalt: {
    label: "Asphalt",
    description: "sealed and fast",
    colour: { light: "#556b82", dark: "#8da8c3" },
  },
  paving: {
    label: "Paving",
    description: "sealed but rough: setts, bricks, paving stones",
    colour: { light: "#7d3ab7", dark: "#b77ff2" },
  },
  compacted: {
    label: "Compacted",
    description: "unpaved, firm enough for road tyres",
    colour: { light: "#009c89", dark: "#59ceba" },
  },
  gravel: {
    label: "Gravel",
    description: "unpaved and loose",
    colour: { light: "#d4a21e", dark: "#fbc959" },
  },
  ground: {
    label: "Ground",
    description: "unpaved and soft: earth, grass, sand, mud",
    colour: { light: "#b23a26", dark: "#f47d67" },
  },
  unknown: {
    label: "Unsurveyed",
    description: "nobody has recorded this stretch — it does not mean smooth",
    colour: { light: "#bbbec1", dark: "#dbdee2" },
  },
};

/** One class's colour for the theme currently on screen. */
export function surfaceColour(kind: SurfaceKind, dark: boolean): string {
  const { light, dark: darkColour } = SURFACE_STYLES[kind].colour;

  return dark ? darkColour : light;
}

/** One stretch of one class, placed along the stage. */
export interface SurfaceBand {
  kind: SurfaceKind;
  startMetres: number;
  endMetres: number;
}

export interface SurfaceShare {
  kind: SurfaceKind;
  metres: number;
  /** Of the whole stage, from 0 to 1. */
  share: number;
}

export interface SurfaceSummary {
  /** Contiguous, in route order, together covering the whole stage. */
  bands: SurfaceBand[];
  /** The classes actually present, in the fixed order of `SURFACE_KINDS`. */
  shares: SurfaceShare[];
  totalMetres: number;
}

/**
 * The part of a range that exists in this coordinate array.
 *
 * Ranges are positions in the geometry they were measured against, and a
 * position outside it is not a small error: `Array.prototype.slice` reads a
 * negative index as a count back from the far end, so an unclamped range would
 * draw or measure a stretch of route the service never classified. Parsing
 * rejects such a range at the API boundary — clamping here keeps a range that
 * got through from placing a class anywhere it does not belong.
 */
function clampRange(range: SurfaceRange, lastIndex: number): { start: number; end: number } {
  const start = Math.min(Math.max(range.startIndex, 0), lastIndex);

  return { start, end: Math.min(Math.max(range.endIndex, start), lastIndex) };
}

/**
 * Places the classification along the stage and totals it by class.
 *
 * Null when there is nothing to describe — too few points, no ranges, or a stage
 * of no length. A share of a zero-length stage is not a smaller truth, it is a
 * division by zero.
 *
 * The stretch between two points is credited to the class of the earlier one, so
 * every metre of the stage is counted exactly once and the shares sum to its
 * length. The alternative — crediting a boundary segment to both sides, as the
 * service's own matched length deliberately does — is right for asking how much
 * was surveyed and wrong for a split that has to add up to a whole.
 */
export function summariseSurface(
  coordinates: Position[],
  ranges: SurfaceRange[],
): SurfaceSummary | null {
  if (coordinates.length < 2 || ranges.length === 0) {
    return null;
  }
  const distances = cumulativeMetres(coordinates);
  const lastIndex = coordinates.length - 1;
  const totalMetres = distances[lastIndex] ?? 0;
  if (totalMetres <= 0) {
    return null;
  }

  const bands: SurfaceBand[] = [];
  const metresByKind = new Map<SurfaceKind, number>();
  for (const range of ranges) {
    const { start, end } = clampRange(range, lastIndex);
    const startMetres = distances[start] ?? 0;
    // One point past the range, because the final point of a range is the first
    // point of the stretch it hands over. The last range has nothing past it.
    const endMetres = distances[Math.min(end + 1, lastIndex)] ?? startMetres;
    if (endMetres <= startMetres) {
      continue;
    }

    bands.push({ kind: range.kind, startMetres, endMetres });
    metresByKind.set(range.kind, (metresByKind.get(range.kind) ?? 0) + (endMetres - startMetres));
  }
  if (bands.length === 0) {
    return null;
  }

  const shares = SURFACE_KINDS.filter((kind) => metresByKind.has(kind)).map((kind) => {
    const metres = metresByKind.get(kind) ?? 0;

    return { kind, metres, share: metres / totalMetres };
  });

  return { bands, shares, totalMetres };
}

/**
 * The class at one distance along the stage, for a readout following the
 * pointer. Distances past the end take the last band rather than nothing, so
 * rounding at the far end of the profile does not blank the readout.
 */
export function surfaceKindAt(summary: SurfaceSummary, metres: number): SurfaceKind | null {
  const band = summary.bands.find((entry) => metres <= entry.endMetres);

  return (band ?? summary.bands[summary.bands.length - 1])?.kind ?? null;
}

/**
 * The classification as it falls inside one stretch of the stage, with the
 * bands at the edges cut to it.
 *
 * A zoomed chart draws its strip from this rather than from the whole stage. A
 * band that begins before the window and ends inside it has to start at the
 * window's edge: drawn from its true start it would run off the left of the
 * plot, and every class boundary after it would land in the wrong place.
 */
export function surfaceBandsWithin(
  summary: SurfaceSummary,
  startMetres: number,
  endMetres: number,
): SurfaceBand[] {
  if (endMetres <= startMetres) {
    return [];
  }

  return summary.bands.flatMap((band) => {
    const start = Math.max(band.startMetres, startMetres);
    const end = Math.min(band.endMetres, endMetres);

    return end > start ? [{ kind: band.kind, startMetres: start, endMetres: end }] : [];
  });
}

/** The drawable geometry of one class, as separate stretches of the route. */
export interface SurfaceLines {
  kind: SurfaceKind;
  lines: Position[][];
}

/** One class's stretches, split by whether the chart is showing them. */
export interface SurfaceSlices {
  kind: SurfaceKind;
  inside: Position[][];
  outside: Position[][];
}

/**
 * Splits the route into one set of lines per class, separating what is drawn at
 * full strength from what is left dim.
 *
 * Each stretch runs one point past its range so neighbouring stretches meet on
 * the shared point, leaving neither a gap in the drawn route nor an overlap that
 * would paint one class over another.
 *
 * The map dims the route outside the stretch the chart is showing, and outside
 * the class a reader has picked out of the key, and `line-opacity` belongs to a
 * layer rather than to a segment — so the two sides have to arrive as different
 * features of the same source, tagged, for one paint expression to tell them
 * apart. Splitting the ranges before they become lines rather than cutting the
 * lines afterwards keeps the clamping and the shared boundary point exactly as
 * they already were.
 */
export function surfaceLinesWithin(
  coordinates: Position[],
  ranges: SurfaceRange[],
  lit: readonly CoordinateRange[] | null,
): SurfaceSlices[] {
  const slicesByKind = new Map<SurfaceKind, { inside: Position[][]; outside: Position[][] }>();
  const lastIndex = coordinates.length - 1;
  if (lastIndex < 1) {
    return [];
  }
  for (const range of ranges) {
    const { start, end } = clampRange(range, lastIndex);
    // One point past the range, because the final point of a range is the first
    // point of the stretch it hands over. The last range has nothing past it.
    const split = splitAtRanges(coordinates, start, Math.min(end + 1, lastIndex), lit);
    if (split.inside.length === 0 && split.outside.length === 0) {
      continue;
    }
    const existing = slicesByKind.get(range.kind);
    if (existing) {
      existing.inside.push(...split.inside);
      existing.outside.push(...split.outside);

      continue;
    }
    slicesByKind.set(range.kind, split);
  }

  return SURFACE_KINDS.flatMap((kind) => {
    const slices = slicesByKind.get(kind);

    return slices ? [{ kind, ...slices }] : [];
  });
}

/** Splits the route into one set of lines per class, ready to paint. */
export function surfaceLines(coordinates: Position[], ranges: SurfaceRange[]): SurfaceLines[] {
  return surfaceLinesWithin(coordinates, ranges, null).map(({ kind, inside }) => ({
    kind,
    lines: inside,
  }));
}
