/**
 * One mix as an upright bar, labelled with the ground each class covers.
 *
 * The route panel stands on a map, so its height is the expensive dimension:
 * read across rather than down, the two mixes cost the card a row instead of
 * two blocks. The bar is read like a cross-section, with the ground that
 * decides the day at the top and the easy majority settling at the bottom.
 *
 * Labels sit beside their own segment rather than in a row underneath, so
 * nothing has to be matched by colour — the name is at the height of the thing
 * it names. That alignment is the whole trick, and the naive form of it
 * destroys what it decorates: a class covering one percent gets a segment two
 * pixels tall, and flooring the rows so its name fits makes every class the
 * same height, at which point the bar is no longer a proportion. So the bar is
 * left exactly proportional and the labels are placed beside it, pushed apart
 * only where they would collide, with a leader line drawn to the segment each
 * one names.
 *
 * Labelled with lengths rather than shares. The bar already *is* the share, so
 * printing it beside it says one thing twice; a distance says a second thing.
 * `13.4 km of gravel` can be pictured and planned around — tyres, time, whether
 * it lands before or after the food stop — where `10%` has to be multiplied by
 * a route length held elsewhere on the card before it means anything.
 */

import type { Highlight } from "../../lib/highlight";
import { sameHighlight } from "../../lib/highlight";
import type { MixEntry } from "../../lib/mix";
import { formatShare } from "../../lib/mix";
import type { UnitSystem } from "../../lib/units";
import { metresToFeet, metresToMiles } from "../../lib/units";
import { HighlightToggle } from "./HighlightToggle";

const COLUMN_HEIGHT = 104;
/**
 * A name at eleven pixels, with just enough air not to touch the one below it.
 *
 * These two set how spread out the column looks. The labels are only pushed
 * apart where they would collide, so the column's own height decides the rest —
 * a tall one spaces the big classes right out and leaves the picture looking
 * airier than the data is sparse. Six classes need ninety pixels to stack
 * without touching, so a hundred and four leaves the proportional spread
 * somewhere to happen and nothing more.
 */
const LABEL_HEIGHT = 15;
const BAR_WIDTH = 8;
const LEADER_WIDTH = 12;

/** Below this many feet, a column reads in feet rather than fractions of a mile. */
const FEET_COLUMN_LIMIT = 5280;

/**
 * One length, in the unit the rest of its column is using.
 *
 * `formatDistance` chooses per value, which is right for a figure standing on
 * its own and wrong for a stack of them: in miles and feet it gives a column
 * reading `3598 ft`, `4707 ft`, `16.5 mi`, in which the largest number names
 * the shortest stretch. So the unit is chosen once, from the longest row, and
 * every row is drawn in it — at the cost of a two-hundred-metre class reading
 * `0.2 km` rather than `200 m`, which is the right way round: a column exists
 * to be compared down, and a class that small is being looked for rather than
 * read off.
 */
function columnLength(metres: number, unitSystem: UnitSystem, longest: number): string {
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

/**
 * Where each label goes, given where its segment is.
 *
 * Each wants to sit level with the middle of its own segment; where two would
 * overlap, the lower one is pushed down and a leader line keeps the
 * correspondence explicit. Six classes at fifteen pixels fit inside the column
 * with room to spare, so the pass never runs out of space.
 */
function placeLabels(shares: number[]): { middle: number; top: number }[] {
  let offset = 0;
  const middles = shares.map((share) => {
    const middle = offset + (share * COLUMN_HEIGHT) / 2;
    offset += share * COLUMN_HEIGHT;

    return middle;
  });

  const tops: number[] = [];
  let lowest = 0;
  for (const middle of middles) {
    const top = Math.max(middle - LABEL_HEIGHT / 2, lowest);
    tops.push(top);
    lowest = top + LABEL_HEIGHT;
  }

  // The downward pass can push the last label off the bottom. Walking back up
  // from the floor gives every label the highest position it can hold without
  // landing on the one below it.
  if (lowest > COLUMN_HEIGHT) {
    let ceiling = COLUMN_HEIGHT;
    for (let index = tops.length - 1; index >= 0; index--) {
      const top = Math.max(Math.min(tops[index] ?? 0, ceiling - LABEL_HEIGHT), 0);
      tops[index] = top;
      ceiling = top;
    }
  }

  return middles.map((middle, index) => ({ middle, top: tops[index] ?? middle }));
}

export interface MixColumnProps {
  name: string;
  /** Canonical order, gentlest and smoothest first — reversed on the way in. */
  entries: MixEntry[];
  /** What to say instead, for a route nothing has classified. */
  absence: string;
  highlight: Highlight | null;
  onHighlightChange: (next: Highlight | null) => void;
  unitSystem: UnitSystem;
}

export function MixColumn({
  name,
  entries,
  absence,
  highlight,
  onHighlightChange,
  unitSystem,
}: MixColumnProps) {
  const stacked = [...entries].reverse();
  const places = placeLabels(stacked.map((entry) => entry.share));
  const longest = Math.max(...stacked.map((entry) => entry.metres), 0);

  return (
    <section className="min-w-0 flex-1">
      <h3 className="mb-1 text-[11px] font-semibold tracking-[0.06em] text-[var(--ink-2)] uppercase">
        {name}
      </h3>
      {stacked.length === 0 ? (
        <p className="text-xs text-[var(--ink-2)]">{absence}</p>
      ) : (
        <div className="relative" style={{ height: COLUMN_HEIGHT }}>
          <div
            className="absolute inset-y-0 left-0 flex flex-col overflow-hidden rounded-sm"
            style={{ width: BAR_WIDTH }}
            aria-hidden="true"
          >
            {stacked.map((entry) => (
              <span
                key={entry.label}
                style={{
                  flexGrow: entry.share,
                  flexBasis: 0,
                  background: entry.colour,
                  opacity:
                    highlight === null || sameHighlight(highlight, entry.highlight) ? 1 : 0.2,
                }}
              />
            ))}
          </div>
          <svg
            className="absolute inset-y-0 overflow-visible"
            style={{ left: BAR_WIDTH, width: LEADER_WIDTH }}
            viewBox={`0 0 ${LEADER_WIDTH} ${COLUMN_HEIGHT}`}
            aria-hidden="true"
          >
            {stacked.map((entry, index) => {
              const place = places[index];
              if (!place) {
                return null;
              }
              const to = place.top + LABEL_HEIGHT / 2;

              return (
                <path
                  key={entry.label}
                  d={`M0 ${place.middle.toFixed(1)} H${LEADER_WIDTH / 2} V${to.toFixed(1)} H${LEADER_WIDTH}`}
                  className="stroke-[var(--rule)]"
                  strokeWidth={1}
                  fill="none"
                />
              );
            })}
          </svg>
          {stacked.map((entry, index) => (
            <div
              key={entry.label}
              className="absolute right-0"
              style={{
                left: BAR_WIDTH + LEADER_WIDTH,
                top: places[index]?.top ?? 0,
                height: LABEL_HEIGHT,
              }}
            >
              <HighlightToggle
                highlight={entry.highlight}
                current={highlight}
                onChange={onHighlightChange}
                // Both quantities spoken whichever one is drawn: the figure on
                // screen is a choice about space, not about what the class is.
                label={`${entry.label}, ${entry.description}, ${columnLength(entry.metres, unitSystem, longest)}, ${formatShare(entry.share)} of the route`}
                title={entry.description}
                className="flex size-full min-w-0 items-baseline gap-1 rounded px-1 text-left text-[11px] leading-none hover:bg-[var(--base)] aria-pressed:bg-[color-mix(in_srgb,var(--accent)_16%,transparent)]"
              >
                <span className="truncate text-[var(--ink-2)]">{entry.label}</span>
                <span className="ml-auto shrink-0 font-semibold tabular-nums">
                  {columnLength(entry.metres, unitSystem, longest)}
                </span>
              </HighlightToggle>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
