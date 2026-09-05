/**
 * A switch for one overlay over the whole map, beside the basemap chooser.
 *
 * A toggle, not a chooser: each overlay is either on the map or not, and two
 * may be on together. They say nothing about the ride — the route's own
 * forecast stays in the dock — so they belong to the map's furniture, where
 * the ground is chosen.
 */

import type { ReactNode } from "react";
import { Button } from "../Button";

export interface OverlayToggleProps {
  on: boolean;
  onChange: (on: boolean) => void;
  icon: ReactNode;
  /** What the overlay shows, lower case, completing "Show the … over the map". */
  subject: string;
  title: string;
}

export function OverlayToggle({ on, onChange, icon, subject, title }: OverlayToggleProps) {
  return (
    <Button
      variant="panel"
      active={on}
      icon={icon}
      onClick={() => onChange(!on)}
      aria-pressed={on}
      aria-label={`${on ? "Hide" : "Show"} the ${subject} over the map`}
      title={title}
    />
  );
}
