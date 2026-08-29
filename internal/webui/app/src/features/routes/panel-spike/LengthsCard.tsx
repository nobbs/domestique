/**
 * **E — Lengths.** The sideways card, labelling each class with its ground.
 *
 * D prints a share beside a bar whose whole job is to draw that share, so the
 * two channels say one thing twice. Swapping the text for a distance makes
 * them say two: the bar carries the proportion, and the words carry what a
 * rider is actually going to ride.
 *
 * The figures answer better questions in this form. `13 km of gravel` is a
 * thing that can be pictured and planned around — tyres, time, whether it
 * falls before or after the food stop — where `10%` has to be multiplied by a
 * route length held somewhere else on the card before it means anything. The
 * thin end improves most: `260 m` is a fact, and the `<1%` it replaces was an
 * admission that the figure had run out of resolution.
 *
 * What it gives up is comparison between routes. A tenth gravel is a tenth
 * gravel on any ride; thirteen kilometres is not, and a reader flicking
 * between routes has to hold each one's total in their head. That is a real
 * loss, and it is smaller than it looks here only because the pill above is
 * carrying the route's distance the whole time.
 */

import type { UnitSystem } from "../../../lib/units";
import { metresToFeet, metresToMiles } from "../../../lib/units";
import { SidewaysCard } from "./SidewaysCard";
import type { CardProps, MixEntry } from "./shared";

/** Below this many feet, a column reads in feet rather than in fractions of a mile. */
const FEET_COLUMN_LIMIT = 5280;

/**
 * One length, in the unit the rest of its column is using.
 *
 * `formatDistance` chooses per value, which is right for a figure standing on
 * its own and wrong for a stack of them: in miles and feet it gave a gradient
 * column reading `3598 ft`, `4707 ft`, `16.5 mi`, in which the largest number
 * names the shortest stretch. So the unit is chosen once, from the longest
 * row, and every row is drawn in it.
 *
 * The cost is at the thin end — a two-hundred-metre class reads `0.2 km`
 * rather than `200 m` — and that is the right way round. A column exists to be
 * compared down, and a class that small is being looked for rather than read
 * off.
 */
function columnLength(metres: number, unitSystem: UnitSystem, column: MixEntry[]): string {
  const longest = Math.max(...column.map((entry) => entry.metres), 0);

  if (unitSystem === "imperial") {
    if (Math.round(metresToFeet(longest)) < FEET_COLUMN_LIMIT) {
      return `${Math.round(metresToFeet(metres))} ft`;
    }

    return `${metresToMiles(metres).toFixed(metresToMiles(longest) < 100 ? 1 : 0)} mi`;
  }

  return longest < 1_000
    ? `${Math.round(metres)} m`
    : `${(metres / 1_000).toFixed(longest < 100_000 ? 1 : 0)} km`;
}

export function LengthsCard(props: CardProps) {
  return (
    <SidewaysCard
      {...props}
      figure={(entry, unitSystem, column) => columnLength(entry.metres, unitSystem, column)}
    />
  );
}
