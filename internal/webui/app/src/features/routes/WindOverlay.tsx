/**
 * The wind over the whole map right now, as streaks drifting through the
 * ICON-D2 grid for the current hour.
 *
 * Deliberately not the ride's forecast. `WindDriftField` drifts along the
 * corridor at the hour the rider reaches each point; this is one hour over all
 * the ground on screen, the way a weather map is. The two are switched on in
 * different places for that reason — this from the map's furniture, that from
 * the dock — and can be on together.
 *
 * Same shader and vertex layout as the corridor field (`windStreakLayer.ts`);
 * only where the particles live and what carries them differ (`windGrid.ts`).
 */

import { useEffect, useMemo, useRef, useState } from "react";
import { Layer, useMap } from "react-map-gl/maplibre";
import { fetchWindGrid } from "../../api/openMeteoGrid";
import { useCartography } from "../../components/map/CartographyContext";
import { PANEL } from "../../lib/cartography";
import { usePrefersReducedMotion } from "../../lib/mediaQuery";
import { FLOATS_PER_VERTEX, mercatorPerPixelAt } from "../../lib/windField";
import type { WindGrid } from "../../lib/windGrid";
import {
  advanceGridField,
  GRID_VERTICES_PER_STREAK,
  seedGridField,
  writeGridStreaks,
} from "../../lib/windGrid";
import { useViewportGrid } from "./useViewportGrid";
import { FIELD_STRENGTH } from "./WindDriftField";
import type { StreakFrame } from "./windStreakLayer";
import { shaderColour, windStreakLayer } from "./windStreakLayer";

export const WIND_OVERLAY_LAYER_ID = "wind-overlay";

const PARTICLES = 2500;
const MAX_FRAME_SECONDS = 0.1;

export interface WindOverlayProps {
  on: boolean;
  /** Hours ahead of the current hour the reader has scrubbed the overlay to. */
  hoursAhead?: number;
  /** The layer the streaks go beneath, so they never cover a route. */
  beforeId?: string | undefined;
}

/** Web Mercator ground resolution at a latitude and zoom, for a 512 px tile. */
function metresPerPixelAt(latitude: number, zoom: number): number {
  return (78271.517 * Math.cos((latitude * Math.PI) / 180)) / 2 ** zoom;
}

export function WindOverlay({ on, hoursAhead = 0, beforeId }: WindOverlayProps) {
  const { dark: darkBasemap } = useCartography();
  const { current: map } = useMap();
  const reducedMotion = usePrefersReducedMotion();
  const wanted = on && !reducedMotion;
  const { data, bboxRef } = useViewportGrid("wind-grid", wanted, fetchWindGrid, hoursAhead);
  // What the frame loop reads: a ref, so a fresh grid changes what the
  // particles drift through without stopping the loop or reseeding them.
  const gridRef = useRef<WindGrid | null>(null);
  gridRef.current = data;

  const inkHex = PANEL[darkBasemap ? "light" : "dark"];
  const colour = useMemo(() => shaderColour(inkHex), [inkHex]);
  const frame = useRef<StreakFrame>({
    vertices: new Float32Array(0),
    vertexCount: 0,
    colour,
    strength: FIELD_STRENGTH,
    primitive: "triangles",
  });
  useEffect(() => {
    frame.current.colour = colour;
  }, [colour]);
  // Made once: `react-map-gl` keeps the first layer it is handed under an id.
  const [streaks] = useState(() => windStreakLayer(WIND_OVERLAY_LAYER_ID, () => frame.current));

  const running = wanted && data !== null;
  // biome-ignore lint/correctness/useExhaustiveDependencies: `bboxRef` is a ref the hook hands back, read on every frame on purpose.
  useEffect(() => {
    if (!running || !map || !bboxRef.current) {
      return;
    }
    const particles = seedGridField(bboxRef.current, PARTICLES);
    const vertices = new Float32Array(PARTICLES * GRID_VERTICES_PER_STREAK * FLOATS_PER_VERTEX);
    frame.current.vertices = vertices;
    let handle: number | null = null;
    let previous: number | null = null;
    const step = (now: number) => {
      handle = requestAnimationFrame(step);
      const seconds = previous === null ? 0 : Math.min((now - previous) / 1000, MAX_FRAME_SECONDS);
      previous = now;
      const field = gridRef.current;
      const within = bboxRef.current;
      if (!field || !within) {
        return;
      }
      const zoom = map.getZoom();
      const metresPerPixel = metresPerPixelAt(map.getCenter().lat, zoom);
      // A particle the pan left outside, or one over ground the old grid does
      // not cover, respawns inside the new viewport on its own.
      advanceGridField(particles, field, within, seconds, metresPerPixel);
      frame.current.vertexCount = writeGridStreaks(
        particles,
        vertices,
        metresPerPixel,
        mercatorPerPixelAt(zoom),
      );
      map.triggerRepaint();
    };
    handle = requestAnimationFrame(step);

    return () => {
      if (handle !== null) {
        cancelAnimationFrame(handle);
      }
      frame.current.vertexCount = 0;
    };
  }, [running, map]);

  if (!running) {
    return null;
  }

  return <Layer {...streaks} {...(beforeId === undefined ? {} : { beforeId })} />;
}
