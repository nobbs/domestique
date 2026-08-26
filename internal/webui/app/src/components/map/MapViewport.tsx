/**
 * The camera, and the one thing every map in this application does with it.
 *
 * Framing is an imperative call on a MapLibre instance rather than a property of
 * the rendered tree, so both maps — the library and the single route — would
 * otherwise carry their own copy of the same effect. They carry this instead.
 */

import { useEffect } from "react";
import { useMap } from "react-map-gl/maplibre";
import type { BoundingBox } from "../../api/types";
import { usePrefersReducedMotion } from "../../lib/mediaQuery";
import type { Insets } from "../../lib/overlayInsets";
import { framePadding, NO_INSETS } from "../../lib/overlayInsets";

export interface MapViewportProps {
  /** What to frame. Null leaves the camera exactly where it is. */
  bounds: BoundingBox | null;
  maxZoom: number;
  /** How much room to leave around the bounds, in pixels. */
  padding?: number;
  /**
   * How much of the pane the panels floating over it are standing on.
   *
   * The camera frames the bounds inside what is left, so a route is framed
   * where the reader can see it rather than half under the column beside it.
   */
  insets?: Insets;
}

/**
 * What each live map was last framed to.
 *
 * The framing below is meant to answer a change of subject and nothing else,
 * but a change of basemap is not one and still reaches it: `MapWidget` holds
 * its children back until the new style has loaded, so this component is
 * unmounted and mounted again, and a fresh mount has no memory of having
 * already framed anything. It would fly back to the bounds it was given and
 * throw away wherever the reader had panned to.
 *
 * Kept against the map's container, which outlives both the style and this
 * component and is released with the map, so the memory lasts exactly as long
 * as the camera it describes.
 */
const framedTo = new WeakMap<object, string>();

export function MapViewport({
  bounds,
  maxZoom,
  padding = 56,
  insets = NO_INSETS,
}: MapViewportProps) {
  const { current: map } = useMap();
  // The camera is animated by MapLibre rather than by a transition, so the
  // stylesheet's reduced-motion block cannot reach it. A reader who asked for
  // less movement gets the new framing outright instead of a flight to it.
  const reducedMotion = usePrefersReducedMotion();
  // Taken apart here rather than passed whole: the insets are measured on every
  // layout, and a camera keyed on the identity of that measurement would fly
  // again on renders that moved nothing.
  const { top, right, bottom, left } = insets;

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
    // Every input the framing is computed from, so a reflow that moves the
    // panels still re-frames while a remount that moved nothing does not.
    const subject = JSON.stringify([bounds, maxZoom, padding, top, right, bottom, left]);
    const container = map.getContainer();
    if (framedTo.get(container) === subject) {
      return;
    }
    framedTo.set(container, subject);

    map.fitBounds(
      [
        [bounds[0], bounds[1]],
        [bounds[2], bounds[3]],
      ],
      {
        padding: framePadding(
          padding,
          { top, right, bottom, left },
          container.clientWidth,
          container.clientHeight,
        ),
        duration: reducedMotion ? 0 : 600,
        maxZoom,
      },
    );
  }, [map, bounds, maxZoom, padding, top, right, bottom, left, reducedMotion]);

  return null;
}
