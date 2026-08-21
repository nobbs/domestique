/**
 * The camera, and the one thing every map in this application does with it.
 *
 * Framing is an imperative call on a MapLibre instance rather than a property of
 * the rendered tree, so both maps — the library and the single route — would
 * otherwise carry their own copy of the same effect. They carry this instead.
 */

import { useEffect } from "react";
import { useMap } from "react-map-gl/maplibre";
import type { BoundingBox } from "../api/types";
import { usePrefersReducedMotion } from "../lib/mediaQuery";

export interface MapViewportProps {
  /** What to frame. Null leaves the camera exactly where it is. */
  bounds: BoundingBox | null;
  maxZoom: number;
  /** How much room to leave around the bounds, in pixels. */
  padding?: number;
}

export function MapViewport({ bounds, maxZoom, padding = 56 }: MapViewportProps) {
  const { current: map } = useMap();
  // The camera is animated by MapLibre rather than by a transition, so the
  // stylesheet's reduced-motion block cannot reach it. A reader who asked for
  // less movement gets the new framing outright instead of a flight to it.
  const reducedMotion = usePrefersReducedMotion();

  useEffect(() => {
    if (!map) {
      return;
    }
    // The map mounts inside a pane whose height is not resolved on the first
    // paint. Observing the container also keeps the canvas correct when the
    // panel beside it reflows at narrow widths.
    const container = map.getContainer();
    map.resize();

    // Without the API the canvas is sized once, on mount, rather than not at
    // all: a map that never reflows still draws.
    if (typeof ResizeObserver === "undefined") {
      return;
    }
    const observer = new ResizeObserver(() => map.resize());
    observer.observe(container);

    return () => observer.disconnect();
  }, [map]);

  useEffect(() => {
    if (!map || !bounds) {
      return;
    }
    // Re-frame when a different route is selected, rather than remounting the
    // map and re-downloading the style — and when a stretch is chosen, so the
    // map shows the ground the chart is showing however the stretch was asked
    // for. Only a change of subject moves the camera: panning away to look at
    // the surrounding roads costs nothing and needs no way back.
    map.fitBounds(
      [
        [bounds[0], bounds[1]],
        [bounds[2], bounds[3]],
      ],
      { padding, duration: reducedMotion ? 0 : 600, maxZoom },
    );
  }, [map, bounds, maxZoom, padding, reducedMotion]);

  return null;
}
