/**
 * The units the map and the panel report distance and elevation in.
 *
 * A chip in the map's own control cluster, next to the basemap chooser and
 * built to the same surface: this is a personal preference over the ground,
 * not a fact about the route. Unlike the basemap there are only two answers,
 * so one press swaps between them rather than opening a list to choose from.
 */

import type { UnitSystem } from "../lib/units";

const NEXT: Record<UnitSystem, UnitSystem> = { metric: "imperial", imperial: "metric" };
const LABEL: Record<UnitSystem, string> = { metric: "km", imperial: "mi" };
const NAME: Record<UnitSystem, string> = { metric: "metric", imperial: "imperial" };

export interface UnitPickerProps {
  system: UnitSystem;
  onSystemChange: (system: UnitSystem) => void;
}

export function UnitPicker({ system, onSystemChange }: UnitPickerProps) {
  const next = NEXT[system];

  return (
    <button
      className="unit-picker"
      type="button"
      aria-label={`Distance and elevation in ${NAME[system]}. Switch to ${NAME[next]}.`}
      onClick={() => onSystemChange(next)}
    >
      {LABEL[system]}
    </button>
  );
}
