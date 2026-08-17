/**
 * A route's shape drawn as a plain SVG polyline.
 *
 * This is deliberately not a map. The library grid renders dozens of these, and
 * a basemap behind each one would be unreadable at this size, slow to load, and
 * would tell the tile provider about every route in the library merely because
 * the operator opened the index. Browsing therefore sends nothing outside the
 * Tailnet; only opening a route loads a basemap.
 */

import { useMemo } from "react";
import type { Position } from "../api/types";

const VIEWBOX = 100;
const PADDING = 8;

/**
 * Projects longitude and latitude onto the viewBox.
 *
 * Longitude is scaled by the cosine of the mean latitude so the shape keeps its
 * real proportions rather than being stretched east-west, and the aspect ratio
 * is preserved by using one scale for both axes.
 */
export function thumbnailPoints(coordinates: Position[]): string {
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

export interface RouteThumbnailProps {
  coordinates: Position[];
  title: string;
}

export function RouteThumbnail({ coordinates, title }: RouteThumbnailProps) {
  const points = useMemo(() => thumbnailPoints(coordinates), [coordinates]);

  if (points === "") {
    return <div className="route-thumbnail route-thumbnail--empty" aria-hidden="true" />;
  }

  return (
    <svg
      className="route-thumbnail"
      viewBox={`0 0 ${VIEWBOX} ${VIEWBOX}`}
      role="img"
      aria-label={`Shape of ${title}`}
      preserveAspectRatio="xMidYMid meet"
    >
      <polyline
        points={points}
        fill="none"
        stroke="currentColor"
        strokeWidth={3}
        strokeLinecap="round"
        strokeLinejoin="round"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  );
}
