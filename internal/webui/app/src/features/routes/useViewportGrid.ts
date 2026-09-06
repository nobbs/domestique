/**
 * A slice of the current hour's model grid, kept up with the viewport.
 *
 * Shared by every overlay that drapes the map: the bbox is the padded view,
 * and the previous slice stays under the overlay while the next loads so a
 * pan never blanks it.
 *
 * Keyed on `gridWindow(bbox)` rather than the bbox itself, rounded or not: the
 * file is read in 32-cell chunks, so two bboxes a few pixels apart usually
 * round to the same chunks and ought to share a fetch, but a plain numeric
 * rounding cannot promise that — its own buckets fall wherever they fall, and
 * a pan that happens to straddle a chunk edge inside one bucket would key the
 * same while the read behind it silently changed. `gridWindow` is the one
 * function that already knows where those edges are.
 */

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { type RefObject, useEffect, useRef, useState } from "react";
import { useMap } from "react-map-gl/maplibre";
import { gridWindow } from "../../api/openMeteoGrid";
import { useHourTick } from "../../lib/clock";
import type { Bbox } from "../../lib/windGrid";

/** The viewport is widened by this share so a pan does not start on empty ground. */
const PAD = 0.25;
/** A grid is asked for again once its hour is this old; the model itself is 3-hourly. */
const GRID_STALE_MS = 15 * 60_000;

function viewBbox(map: ReturnType<typeof useMap>["current"]): Bbox | null {
  // The unit tests' map double has no bounds to give.
  const bounds = typeof map?.getBounds === "function" ? map.getBounds() : null;
  if (!bounds) {
    return null;
  }
  const west = bounds.getWest();
  const south = bounds.getSouth();
  const east = bounds.getEast();
  const north = bounds.getNorth();
  const padLon = (east - west) * PAD;
  const padLat = (north - south) * PAD;

  return [west - padLon, south - padLat, east + padLon, north + padLat];
}

export interface ViewportGrid<T> {
  data: T | null;
  /** The bbox as the frame loop should read it: current without a render. */
  bboxRef: RefObject<Bbox | null>;
}

export function useViewportGrid<T>(
  key: string,
  on: boolean,
  read: (bbox: Bbox, at: Date) => Promise<T | null>,
  /** Hours ahead of the current hour to read, for a reader scrubbing the forecast forward. */
  hoursAhead = 0,
): ViewportGrid<T> {
  const { current: map } = useMap();
  const [bbox, setBbox] = useState<Bbox | null>(null);
  const bboxRef = useRef<Bbox | null>(null);
  // Nothing else re-renders this hook on the hour: `hourKey` below is read
  // straight off the clock, so an overlay left open across an hour boundary
  // with no pan and no toggle would otherwise keep asking for the hour that
  // was current when it last rendered.
  useHourTick(on);

  useEffect(() => {
    // Every overlay calls this hook whether or not a reader has switched it
    // on, so an unconditional subscription here would be four or five sets
    // of map listeners doing work on every pan for overlays drawing nothing.
    if (!on || !map || typeof map.on !== "function") {
      bboxRef.current = null;
      setBbox(null);

      return;
    }
    // Live on every frame of the pan: a frame loop reading `bboxRef` respawns
    // particles into the view actually on screen, not the one before the pan
    // started. `setBbox` — which triggers a fetch — waits for `moveend`
    // instead, so a pan mid-flight never queues a slice for ground the reader
    // has already scrolled past.
    const track = () => {
      bboxRef.current = viewBbox(map);
    };
    const settle = () => {
      track();
      setBbox(bboxRef.current);
    };
    settle();
    map.on("move", track);
    map.on("moveend", settle);

    return () => {
      map.off("move", track);
      map.off("moveend", settle);
    };
  }, [on, map]);

  // Floored, not rounded: rounding would flip to the next hour at :30, jumping
  // the fetched data — and the picker's own label — half an hour early.
  const hourKey = Math.floor(Date.now() / 3_600_000) + hoursAhead;
  const bboxKey = bbox ? gridWindow(bbox) : null;
  const grid = useQuery({
    queryKey: [key, hourKey, bboxKey],
    queryFn: () => (bbox ? read(bbox, new Date(hourKey * 3_600_000)) : null),
    enabled: on && bbox !== null,
    staleTime: GRID_STALE_MS,
    // Only while on: `keepPreviousData` carries a query's last data forward
    // for a caller with no live fetch behind it at all, disabled or not, so
    // an overlay switched off would otherwise keep reporting the slice from
    // before — never the `null` its own consumers' cleanup depends on. Left
    // out entirely rather than set to `undefined`, which the option's own
    // type refuses under this project's strict optional properties.
    ...(on ? { placeholderData: keepPreviousData } : {}),
  });

  return { data: grid.data ?? null, bboxRef };
}
