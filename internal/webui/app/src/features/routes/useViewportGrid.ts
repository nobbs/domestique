/**
 * A slice of the current hour's model grid, kept up with the viewport.
 *
 * Shared by every overlay that drapes the map: the bbox is the padded view,
 * rounded so a small pan re-reads nothing, and the previous slice stays under
 * the overlay while the next loads so a pan never blanks it.
 */

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { type RefObject, useEffect, useRef, useState } from "react";
import { useMap } from "react-map-gl/maplibre";
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
): ViewportGrid<T> {
  const { current: map } = useMap();
  const [bbox, setBbox] = useState<Bbox | null>(null);
  const bboxRef = useRef<Bbox | null>(null);

  useEffect(() => {
    if (!map || typeof map.on !== "function") {
      return;
    }
    const update = () => {
      bboxRef.current = viewBbox(map);
      setBbox(bboxRef.current);
    };
    update();
    map.on("moveend", update);

    return () => {
      map.off("moveend", update);
    };
  }, [map]);

  // The hour on the clock, which is what "now" means to a reader.
  const hourKey = Math.round(Date.now() / 3_600_000);
  const bboxKey = bbox ? bbox.map((value) => Math.round(value * 10) / 10) : null;
  const grid = useQuery({
    queryKey: [key, hourKey, bboxKey],
    queryFn: () => (bbox ? read(bbox, new Date(hourKey * 3_600_000)) : null),
    enabled: on && bbox !== null,
    staleTime: GRID_STALE_MS,
    placeholderData: keepPreviousData,
  });

  return { data: grid.data ?? null, bboxRef };
}
