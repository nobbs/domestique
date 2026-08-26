import { IconMapPinFilled } from "@tabler/icons-react";

/**
 * The "you are here" pin, with no map under it.
 *
 * Split from `MapControls` because the two halves need entirely different
 * things to be seen: the `Marker` that puts this somewhere is react-map-gl's
 * and wants a live camera, while the pin itself is 28 pixels of CSS that wants
 * nothing at all. Kept apart, the part worth looking at can be looked at —
 * `MapControls` stays where the map-shaped half belongs, mocked, in Testing
 * Library.
 *
 * Pointer events are off in the stylesheet: it floats over the map a reader is
 * dragging, and a pin that swallowed the cursor would stall the gesture.
 */
export function LocationPin() {
  return (
    <div className="current-location-marker" role="img" aria-label="Your location">
      <IconMapPinFilled size={18} aria-hidden="true" />
    </div>
  );
}
