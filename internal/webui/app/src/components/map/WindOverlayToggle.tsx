/**
 * The switch for the wind over the whole map, beside the basemap chooser.
 *
 * A toggle, not a chooser: there is one overlay and it is either on the map or
 * not. It says nothing about the ride — the route's own forecast stays in the
 * dock — so it belongs to the map's furniture, where the ground is chosen.
 */

import { IconWind } from "@tabler/icons-react";
import { Button } from "../Button";

export interface WindOverlayToggleProps {
  on: boolean;
  onChange: (on: boolean) => void;
}

export function WindOverlayToggle({ on, onChange }: WindOverlayToggleProps) {
  return (
    <Button
      variant="panel"
      active={on}
      icon={<IconWind stroke={1.6} />}
      onClick={() => onChange(!on)}
      aria-pressed={on}
      aria-label={on ? "Hide the wind over the map" : "Show the wind over the map"}
      title="Wind now, from ICON-D2"
    />
  );
}
