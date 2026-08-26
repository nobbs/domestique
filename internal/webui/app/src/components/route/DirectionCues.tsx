import { IconChevronsRight } from "@tabler/icons-react";
import { useEffect, useMemo, useState } from "react";
import { Marker, useMap } from "react-map-gl/maplibre";
import type { Position } from "../../api/types";
import { bearingBetween, directionChevrons, metresPerPixel } from "../../lib/routeCues";

/**
 * Chevrons along the route, pointing the way it is ridden.
 *
 * A child of the map because the cues are sized and spaced in screen pixels, and
 * only the live camera can say what a pixel is worth on the ground. They are
 * rebuilt when the camera settles rather than on every frame of a flight: the
 * geometry is measured over the whole route, and a stage of several thousand
 * points would otherwise be re-measured sixty times a second to no visible end.
 */
export function DirectionCues({
  coordinates,
  darkBasemap,
  color,
}: {
  coordinates: Position[];
  darkBasemap: boolean;
  /** The route's casing colour for this basemap — see `RouteOverlay`'s `ROUTE_CASING`. */
  color: string;
}) {
  const { current: map } = useMap();
  const [resolution, setResolution] = useState<number | null>(null);

  useEffect(() => {
    if (!map) {
      return;
    }
    const read = () => setResolution(metresPerPixel(map.getZoom(), map.getCenter().lat));
    read();
    map.on("moveend", read);
    map.on("zoomend", read);

    return () => {
      map.off("moveend", read);
      map.off("zoomend", read);
    };
  }, [map]);

  const chevrons = useMemo(
    () =>
      resolution === null ? [] : directionChevrons(coordinates, { metresPerPixel: resolution }),
    [coordinates, resolution],
  );
  // A marker sits at each chevron tip, rotated from its two arms rather than
  // from a bearing guessed again from the route's raw points.
  const markers = useMemo(
    () =>
      chevrons.flatMap((chevron, index) => {
        const [left, tip, right] = chevron;
        if (!left || !tip || !right) {
          return [];
        }
        const behind: Position = [(left[0] + right[0]) / 2, (left[1] + right[1]) / 2];

        return [{ key: index, position: tip, rotation: bearingBetween(behind, tip) - 90 }];
      }),
    [chevrons],
  );

  return (
    <>
      {markers.map(({ key, position, rotation }) => (
        <Marker
          key={key}
          longitude={position[0]}
          latitude={position[1]}
          anchor="center"
          className="route-direction-marker"
        >
          <IconChevronsRight
            className={`route-direction route-direction--${darkBasemap ? "dark" : "light"}`}
            color={color}
            size={26}
            stroke={3}
            style={{ transform: `rotate(${rotation}deg)` }}
            aria-hidden="true"
          />
        </Marker>
      ))}
    </>
  );
}
