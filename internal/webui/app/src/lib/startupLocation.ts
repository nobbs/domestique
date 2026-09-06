/**
 * The rider's own position, read once at startup to frame the first view.
 *
 * The permission prompt and the fix it waits for are both outside this app's
 * control and sometimes slow or refused, so the request never blocks
 * rendering: the caller's default framing stands until a position arrives, if
 * it ever does. Nothing here is stored or sent anywhere — the coordinates live
 * only in the caller's own state for as long as the page is open.
 */

import { useEffect, useState } from "react";
import type { BoundingBox } from "../api/types";

/**
 * The zoom a rider's own position is shown at: the closest the startup framing
 * comes, and the least the locate button zooms in to.
 */
export const LOCATION_ZOOM = 12;

/** `[longitude, latitude]` once the browser answers; null while waiting, denied, unavailable, or timed out. */
export function useStartupLocation(enabled: boolean): [number, number] | null {
  const [position, setPosition] = useState<[number, number] | null>(null);

  useEffect(() => {
    const geolocation = typeof navigator === "undefined" ? undefined : navigator.geolocation;
    if (!enabled || typeof geolocation?.getCurrentPosition !== "function") {
      return;
    }
    let cancelled = false;
    geolocation.getCurrentPosition(
      ({ coords }) => {
        if (!cancelled) {
          setPosition([coords.longitude, coords.latitude]);
        }
      },
      () => {
        // Denied, unavailable, or timed out: the caller's default framing stands.
      },
      { timeout: 8000, maximumAge: 300_000 },
    );

    return () => {
      cancelled = true;
    };
  }, [enabled]);

  return enabled ? position : null;
}

/** A small box around one point, for framing a position with no route to fit. */
export function boxAround([longitude, latitude]: [number, number], degrees = 0.01): BoundingBox {
  return [longitude - degrees, latitude - degrees, longitude + degrees, latitude + degrees];
}
