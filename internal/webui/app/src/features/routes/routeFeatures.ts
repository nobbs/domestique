/**
 * The route's geometry as the features a map layer paints, and how they dim.
 *
 * Shared between the layers `RouteOverlay` mounts itself and the ones its
 * children mount into the same stack: what stays lit is one answer for the
 * whole overlay, so the expression that reads it has to be one copy rather
 * than one per component that draws along the route.
 */

import type { DataDrivenPropertyValueSpecification } from "maplibre-gl";
import type { Position } from "../../api/types";

/**
 * How much of the route survives outside the ground on show.
 *
 * Dimmed rather than hidden: the ride does not stop at the edges of a window,
 * and a route drawn only in the middle would read as a shorter route rather than
 * as a closer look at a longer one. The same holds for a class picked out of the
 * key — the gravel is somewhere on this ride, and hiding the tarmac either side
 * of it would lose where. A quarter is faint enough that the eye lands on the
 * lit ground first and still dark enough to follow the road between.
 */
export const OUTSIDE_OPACITY = 0.25;

/**
 * How wide an edging runs beneath the casing.
 *
 * Wider than the casing, so what shows is an edging either side of it rather
 * than a colour behind it. The casing keeps the encodings apart: whatever is
 * edged here never touches the class colour it would otherwise be read as part
 * of. One slot, so only one thing is ever edged at a time — steepness usually,
 * the rider-relative wind while that is what the reader asked for.
 */
export const EDGING_WIDTH = 11;

/**
 * An opacity that drops away outside the ground on show.
 *
 * One expression over one tagged source rather than a second stack of layers:
 * `line-opacity` is a layer property, and one layer per class is exactly what
 * keeps a class's dash pattern identical on both sides of a lit stretch's edge.
 * Written as a function because a bare array in a `const` widens to `unknown[]`
 * and stops matching the tuple union the paint property is typed as.
 */
export function dimmedOutside(
  full: number,
  dimmed: boolean,
): DataDrivenPropertyValueSpecification<number> {
  return dimmed ? ["case", ["get", "shown"], full, full * OUTSIDE_OPACITY] : full;
}

/**
 * The route's geometry as at most two features, tagged by whether the chart is
 * showing them, so one paint expression can tell the two apart.
 *
 * `properties` carries anything else the layer reads off the feature — a
 * colour, where one layer paints stretches of several.
 */
export function taggedCollection(
  slices: { inside: Position[][]; outside: Position[][] },
  properties: Record<string, unknown> = {},
) {
  const features = ([lines, shown]: [Position[][], boolean]) =>
    lines.length === 0
      ? []
      : [
          {
            type: "Feature" as const,
            geometry: { type: "MultiLineString" as const, coordinates: lines },
            properties: { ...properties, shown },
          },
        ];

  return {
    type: "FeatureCollection" as const,
    features: [...features([slices.inside, true]), ...features([slices.outside, false])],
  };
}
