/**
 * Presentation of a stage's surface classification.
 *
 * The service reports which stretches of a stage are sealed, firm, loose, or
 * soft. This module decides how that reads: the palette and dash pattern each
 * class wears, and the measurements the map, the legend, and the elevation
 * readout all take from one place so they cannot disagree.
 *
 * Lengths are measured here rather than taken from the API because a share needs
 * metres per class, and the API reports one total. The spherical model is shared
 * with the elevation profile, so the figures agree with the distance shown beside
 * them.
 */

import type { Position, SurfaceKind, SurfaceRange } from "../api/types";
import { SURFACE_KINDS } from "../api/types";
import { haversineMetres } from "./profile";

interface SurfaceStyle {
  label: string;
  /** What the class means, for a legend that has to explain "compacted". */
  description: string;
  colour: string;
  /**
   * The MapLibre `line-dasharray`, in multiples of the line width. Empty is a
   * solid line.
   */
  dashes: number[];
}

/**
 * The width the classified route is drawn at, in pixels.
 *
 * It is the unit `dashes` are counted in, which is why a legend swatch scales its
 * pattern by the same number: the swatch is then the map at actual size rather
 * than an approximation of it.
 */
export const SURFACE_LINE_WIDTH = 4;

/**
 * How each class is drawn.
 *
 * Two channels carry the class, because either one alone fails somebody. Hue
 * separates sealed ground (cool) from unsealed (warm earth tones) and leaves
 * unsurveyed neutral, which is the distinction a rider cares about first. The
 * dash pattern then runs from solid to sparse dots as the ground gets looser: an
 * ordered channel for an ordered measure, and the one that survives colour
 * blindness, greyscale, and a basemap that happens to share a hue.
 *
 * The colours are literals rather than the CSS custom properties the rest of the
 * UI uses, because MapLibre paints on a canvas and cannot resolve a variable.
 * They are mid-tone on purpose: the same value has to hold up over a pale
 * basemap and a dark one.
 *
 * Unsurveyed is deliberately the quietest thing on the map. Fine grey ticks on
 * the white casing say the route goes here and nobody has recorded what it is
 * made of, which is all that is actually known.
 *
 * The dashes are drawn with butt caps, so these lengths are what appears. Round
 * caps would extend every dash by a full line width, which closes the short
 * patterns up into each other and undoes the ordering.
 */
export const SURFACE_STYLES: Record<SurfaceKind, SurfaceStyle> = {
  asphalt: {
    label: "Asphalt",
    description: "sealed and fast",
    colour: "#1F5FA8",
    dashes: [],
  },
  paving: {
    label: "Paving",
    description: "sealed but rough: setts, bricks, paving stones",
    colour: "#6C4FA0",
    dashes: [3, 0.6],
  },
  compacted: {
    label: "Compacted",
    description: "unpaved, firm enough for road tyres",
    colour: "#7E8B33",
    dashes: [2.4, 1.2],
  },
  gravel: {
    label: "Gravel",
    description: "unpaved and loose",
    colour: "#B5822E",
    dashes: [1.4, 1.2],
  },
  ground: {
    label: "Ground",
    description: "unpaved and soft: earth, grass, sand, mud",
    colour: "#6B4423",
    dashes: [0.6, 1],
  },
  unknown: {
    label: "Unsurveyed",
    description: "nobody has recorded this stretch — it does not mean smooth",
    colour: "#8C8C94",
    dashes: [0.5, 2.6],
  },
};

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

/** Distance from the start of the stage to each point. */
function cumulativeMetres(coordinates: Position[]): number[] {
  const distances = [0];
  for (let index = 1; index < coordinates.length; index++) {
    const previous = coordinates[index - 1];
    const current = coordinates[index];
    const travelled = distances[index - 1] ?? 0;
    distances.push(
      previous && current ? travelled + haversineMetres(previous, current) : travelled,
    );
  }

  return distances;
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

/** The drawable geometry of one class, as separate stretches of the route. */
export interface SurfaceLines {
  kind: SurfaceKind;
  lines: Position[][];
}

/**
 * Splits the route into one set of lines per class, ready to paint.
 *
 * Each stretch runs one point past its range so neighbouring stretches meet on
 * the shared point, leaving neither a gap in the drawn route nor an overlap that
 * would paint one class over another. A range covering a single final point
 * spans no ground and is dropped: there is no line to draw through one point.
 */
export function surfaceLines(coordinates: Position[], ranges: SurfaceRange[]): SurfaceLines[] {
  const linesByKind = new Map<SurfaceKind, Position[][]>();
  const lastIndex = coordinates.length - 1;
  if (lastIndex < 1) {
    return [];
  }
  for (const range of ranges) {
    const { start, end } = clampRange(range, lastIndex);
    const line = coordinates.slice(start, end + 2);
    if (line.length < 2) {
      continue;
    }
    const existing = linesByKind.get(range.kind);
    if (existing) {
      existing.push(line);

      continue;
    }
    linesByKind.set(range.kind, [line]);
  }

  return SURFACE_KINDS.flatMap((kind) => {
    const lines = linesByKind.get(kind);

    return lines ? [{ kind, lines }] : [];
  });
}

/**
 * A CSS background echoing how the class is drawn on the map, for a swatch.
 *
 * A legend of flat colour chips would teach half of what the map shows, and the
 * half that fails in greyscale at that. The gaps are the same colour at low
 * alpha rather than transparent, so the swatch keeps its shape against whatever
 * sits behind it.
 */
export function swatchBackground(kind: SurfaceKind): string {
  const style = SURFACE_STYLES[kind];
  const [dash, gap] = style.dashes;
  if (dash === undefined || gap === undefined) {
    return style.colour;
  }
  const dashWidth = dash * SURFACE_LINE_WIDTH;
  const period = dashWidth + gap * SURFACE_LINE_WIDTH;

  return (
    `repeating-linear-gradient(90deg, ${style.colour} 0 ${dashWidth}px, ` +
    `${style.colour}33 ${dashWidth}px ${period}px)`
  );
}
