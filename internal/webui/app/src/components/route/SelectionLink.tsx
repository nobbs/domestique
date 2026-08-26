import { useEffect } from "react";
import { useMap } from "react-map-gl/maplibre";
import { routeSelection } from "../../lib/mapSelection";
import type { DistanceWindow, Profile } from "../../lib/profile";

/**
 * Lets a drag along the painted route pick the stretch it covers.
 *
 * A child of the map for the same reason the hover link is: `useMap` is what
 * resolves the instance, and the gesture is judged in pixels against the
 * projected route, which needs the live camera. The stretch under the hand is
 * reported back while it is still being drawn, so the map lights it as it goes
 * — the same running answer the chart gives a drag across its own plot.
 */
export function SelectionLink({
  profile,
  onPending,
  onZoomChange,
}: {
  profile: Profile | null;
  onPending: (window: DistanceWindow | null) => void;
  onZoomChange: ((window: DistanceWindow | null) => void) | undefined;
}) {
  const { current: map } = useMap();

  useEffect(() => {
    if (!map || !profile || !onZoomChange) {
      return;
    }

    return routeSelection(map.getMap(), { profile, onPending, onSelect: onZoomChange });
  }, [map, profile, onPending, onZoomChange]);

  return null;
}
