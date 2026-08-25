/**
 * A route's shape drawn as a plain SVG polyline.
 *
 * This is deliberately not a map. The results column renders one of these per
 * row, and a basemap behind each would be unreadable at thirty pixels, slow to
 * load, and would tell the tile provider about every route in the library merely
 * because the operator opened the index. Browsing therefore sends nothing
 * outside the Tailnet; the cartography on the page is one map, drawn once.
 *
 * The glyph is stroked in the route's steepest gradient band, so a row says how
 * hard the ride is before its figures are read — the same ramp the chart and the
 * map band with, at full strength because this is a small mark.
 */

import { useMemo } from "react";
import type { Position } from "../api/types";

/**
 * The viewBox, in the units the stroke is measured against.
 *
 * Small on purpose: the glyph is drawn at 30 px in a row, and a coarse box keeps
 * the shape from resolving into detail nobody can see at that size.
 */
const VIEWBOX = 48;
const PADDING = 4;
const bandStroke = [
  "stroke-[var(--grade-0)]",
  "stroke-[var(--grade-1)]",
  "stroke-[var(--grade-2)]",
  "stroke-[var(--grade-3)]",
  "stroke-[var(--grade-4)]",
] as const;

/**
 * Projects longitude and latitude onto the viewBox.
 *
 * Longitude is scaled by the cosine of the mean latitude so the shape keeps its
 * real proportions rather than being stretched east-west, and the aspect ratio
 * is preserved by using one scale for both axes.
 */
export function glyphPoints(coordinates: Position[]): string {
  if (coordinates.length < 2) {
    return "";
  }

  const meanLatitude =
    coordinates.reduce((total, [, latitude]) => total + latitude, 0) / coordinates.length;
  const longitudeScale = Math.cos((meanLatitude * Math.PI) / 180);

  const projected = coordinates.map(([longitude, latitude]) => ({
    x: longitude * longitudeScale,
    y: -latitude,
  }));

  const xs = projected.map((point) => point.x);
  const ys = projected.map((point) => point.y);
  const minX = Math.min(...xs);
  const maxX = Math.max(...xs);
  const minY = Math.min(...ys);
  const maxY = Math.max(...ys);

  const span = Math.max(maxX - minX, maxY - minY);
  const usable = VIEWBOX - 2 * PADDING;
  // A route that is a single point has no span; centre it rather than divide by zero.
  const scale = span > 0 ? usable / span : 0;
  const offsetX = PADDING + (usable - (maxX - minX) * scale) / 2;
  const offsetY = PADDING + (usable - (maxY - minY) * scale) / 2;

  return projected
    .map((point) => {
      const x = offsetX + (point.x - minX) * scale;
      const y = offsetY + (point.y - minY) * scale;
      return `${x.toFixed(2)},${y.toFixed(2)}`;
    })
    .join(" ");
}

export interface RouteGlyphProps {
  coordinates: Position[];
  title: string;
  /** The route's steepest gradient band, which the shape is stroked in. */
  band: number;
}

export function RouteGlyph({ coordinates, title, band }: RouteGlyphProps) {
  const points = useMemo(() => glyphPoints(coordinates), [coordinates]);

  if (points === "") {
    return <div className="size-full rounded bg-[var(--base)]" role="presentation" />;
  }

  return (
    <svg
      className="block size-full"
      viewBox={`0 0 ${VIEWBOX} ${VIEWBOX}`}
      role="img"
      aria-label={`Shape of ${title}`}
      preserveAspectRatio="xMidYMid meet"
    >
      <polyline
        // The band is carried as data rather than as a stroke, because the ramp
        // is a set of custom properties and a presentation attribute cannot
        // resolve one — see the `.route-glyph` rules in index.css.
        data-band={band}
        className={bandStroke[band] ?? bandStroke[0]}
        points={points}
        fill="none"
        strokeWidth={2}
        strokeLinecap="round"
        strokeLinejoin="round"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  );
}
