import { useEffect } from "react";
import { useMap } from "react-map-gl/maplibre";
import type { Profile } from "../../lib/profile";
import { nearestSample } from "../../lib/profile";
import { NEAR_ROUTE_PIXELS } from "../../lib/selection";

/**
 * Reports the position under the pointer, so the elevation chart can mark it.
 *
 * This is a child of the map because `useMap` is what resolves the instance,
 * and projecting the candidate sample back to the screen is the only way to
 * judge nearness in the units the pointer actually moves in.
 */
export function HoverLink({
  profile,
  onActiveChange,
}: {
  profile: Profile | null;
  onActiveChange: ((metres: number | null) => void) | undefined;
}) {
  const { current: map } = useMap();

  useEffect(() => {
    if (!map || !profile || !onActiveChange) {
      return;
    }

    const onMove = (event: {
      lngLat: { lng: number; lat: number };
      point: { x: number; y: number };
    }) => {
      const index = nearestSample(profile, event.lngLat.lng, event.lngLat.lat);
      const sample = index === null ? undefined : profile.samples[index];
      if (!sample) {
        onActiveChange(null);

        return;
      }
      const projected = map.project([sample.longitude, sample.latitude]);
      const near =
        Math.hypot(projected.x - event.point.x, projected.y - event.point.y) <= NEAR_ROUTE_PIXELS;
      // Reported as a distance along the route, which is the one unit that means
      // the same ground to this map and to a chart showing any stretch of it.
      onActiveChange(near ? sample.distanceMetres : null);
    };
    const onLeave = () => onActiveChange(null);

    map.on("mousemove", onMove);
    map.on("mouseout", onLeave);

    return () => {
      map.off("mousemove", onMove);
      map.off("mouseout", onLeave);
    };
  }, [map, profile, onActiveChange]);

  return null;
}
