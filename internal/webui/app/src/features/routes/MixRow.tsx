/**
 * One mix as a bar across, labelled with the ground each class covers.
 *
 * Two of these make the card's mix: gradient's tags above its bar, surface's
 * below its own, so the bars meet in the middle and read as one bar split by a
 * hairline. Upright, each mix had about half the card's width and spent most
 * of it on the gap between a bar and its labels; across, each gets the whole
 * width and the segments are drawn at the size they actually differ by.
 *
 * Tags sit under — or over — their own segment rather than in a key elsewhere,
 * so nothing has to be matched by colour. That alignment is the whole trick,
 * and the naive form of it destroys what it decorates: a class covering one
 * percent gets a segment three pixels wide, and widening the segments so the
 * name fits makes every class the same size, at which point the bar is no
 * longer a proportion. So the bar is left exactly proportional and the tags are
 * placed along it, pushed apart only where they would collide, with a leader
 * drawn to the segment each one names.
 *
 * Labelled with lengths rather than shares. The bar already *is* the share, so
 * printing it beside it says one thing twice; a distance says a second thing.
 * `13.4 km of gravel` can be pictured and planned around — tyres, time, whether
 * it lands before or after the food stop — where `10%` has to be multiplied by
 * a route length held elsewhere on the card before it means anything.
 *
 * Sorted by size rather than by where the ground is. The dock's ribbon is the
 * positional reading of the same classes, and two positional bars would be the
 * same picture twice; this one answers how much, which the ribbon cannot.
 */

import type { Highlight } from "../../lib/highlight";
import { sameHighlight } from "../../lib/highlight";
import type { MixEntry } from "../../lib/mix";
import { formatShare } from "../../lib/mix";
import type { UnitSystem } from "../../lib/units";
import { metresToFeet, metresToMiles } from "../../lib/units";
import { useElementWidth } from "../../lib/useElementWidth";
import { HighlightToggle } from "./HighlightToggle";

const BAR_HEIGHT = 10;
/** The drop a leader falls through, between the bar and its tags. */
const LEADER_HEIGHT = 12;
/**
 * One tag's slot, and the height of the two lines inside it.
 *
 * Fixed, for the reason the upright bar's row height was: the placement has to
 * reserve a size before it knows what the text comes to, and a slot measured
 * per tag would let one long class name shove its neighbours along. Wide enough
 * for `Compacted` over `10.2 km` at these sizes.
 */
const TAG_WIDTH = 62;
const TAG_HEIGHT = 26;
/** Clearance between two pushed-apart tags, so their text never runs together. */
const TAG_GAP = 6;

/** The card's own content width, until the bar has been measured. */
const ASSUMED_WIDTH = 344;

/** Below this many feet, a bar reads in feet rather than fractions of a mile. */
const FEET_LIMIT = 5280;

/**
 * One length, in the unit its bar is using.
 *
 * `formatDistance` chooses per value, which is right for a figure standing on
 * its own and wrong for a row of them: in miles and feet it gives a bar reading
 * `3598 ft`, `4707 ft`, `16.5 mi`, in which the largest number names the
 * shortest stretch. So imperial picks its unit once, from the longest, and
 * draws every tag in it. Metric has no such ft/mi split — only a decimal
 * place that gets coarser once the longest is over 100 km — so a short tag
 * still reads in metres beside a row of kilometres.
 */
function barLength(metres: number, unitSystem: UnitSystem, longest: number): string {
  if (unitSystem === "imperial") {
    if (Math.round(metresToFeet(longest)) < FEET_LIMIT) {
      return `${Math.round(metresToFeet(metres))} ft`;
    }

    return `${metresToMiles(metres).toFixed(metresToMiles(longest) < 100 ? 1 : 0)} mi`;
  }

  if (metres < 1_000) {
    return `${Math.round(metres)} m`;
  }

  return `${(metres / 1_000).toFixed(longest < 100_000 ? 1 : 0)} km`;
}

/**
 * Where each tag goes, given where its segment is.
 *
 * Each wants to sit under the middle of its own segment; where two would
 * overlap, the later one is pushed along and a leader keeps the correspondence
 * explicit. The tags are wider than most segments, so this pass does real work
 * rather than tidying an edge case — the leaders are load-bearing.
 */
export function placeTags(shares: number[], extent: number): { middle: number; left: number }[] {
  let offset = 0;
  const middles = shares.map((share) => {
    const middle = offset + (share * extent) / 2;
    offset += share * extent;

    return middle;
  });

  const lefts: number[] = [];
  let rightmost = 0;
  for (const [index, middle] of middles.entries()) {
    const left = Math.max(middle - TAG_WIDTH / 2, rightmost);
    lefts.push(left);
    rightmost = left + TAG_WIDTH + (index < middles.length - 1 ? TAG_GAP : 0);
  }

  // The forward pass can push the last tag off the end. Walking back from the
  // right edge gives every tag the earliest place it can hold without landing
  // on the one after it.
  if (rightmost > extent) {
    let wall = extent;
    for (let index = lefts.length - 1; index >= 0; index--) {
      const left = Math.max(Math.min(lefts[index] ?? 0, wall - TAG_WIDTH), 0);
      lefts[index] = left;
      wall = left - (index > 0 ? TAG_GAP : 0);
    }
  }

  return middles.map((middle, index) => ({ middle, left: lefts[index] ?? middle }));
}

export interface MixRowProps {
  /**
   * What the classes are called collectively, for the list that holds them.
   *
   * The row draws no heading: five percentages say "gradient" and five surface
   * names say "surface" without being told, and a heading apiece was two rows
   * spent restating the content. This is what remains for anything reading the
   * card rather than looking at it.
   */
  classesLabel: string;
  /** The classes this route has, largest share first. */
  entries: MixEntry[];
  /** What to say instead, for a route nothing has classified. */
  absence: string;
  /**
   * Which side of the bar the tags hang off.
   *
   * The pair is drawn mirrored, so the two bars meet with nothing between them
   * and the labelling fans outward. The bar takes the far edge from its tags,
   * which is also the edge that gets the pair's rounding — so neither row has
   * to be told which of the two it is.
   */
  tagSide: "above" | "below";
  highlight: Highlight | null;
  onHighlightChange: (next: Highlight | null) => void;
  unitSystem: UnitSystem;
}

export function MixRow({
  classesLabel,
  entries,
  absence,
  tagSide,
  highlight,
  onHighlightChange,
  unitSystem,
}: MixRowProps) {
  const above = tagSide === "above";
  const { ref, width } = useElementWidth<HTMLDivElement>();
  const extent = width > 0 ? width : ASSUMED_WIDTH;
  const longest = Math.max(...entries.map((entry) => entry.metres), 0);
  const places = placeTags(
    entries.map((entry) => entry.share),
    extent,
  );

  if (entries.length === 0) {
    return <p className="text-xs text-[var(--ink-2)]">{absence}</p>;
  }

  return (
    <div ref={ref} className="relative" style={{ height: BAR_HEIGHT + LEADER_HEIGHT + TAG_HEIGHT }}>
      <div
        // Rounded on the outside only, so the pair reads as one bar split by a
        // hairline rather than as two stacked pills.
        className={`absolute inset-x-0 flex overflow-hidden ${above ? "rounded-t-md" : "rounded-b-md"}`}
        style={{ height: BAR_HEIGHT, ...(above ? { bottom: 0 } : { top: 0 }) }}
        aria-hidden="true"
      >
        {entries.map((entry) => (
          <span
            key={entry.label}
            style={{
              flexGrow: entry.share,
              flexBasis: 0,
              background: entry.colour,
              opacity: highlight === null || sameHighlight(highlight, entry.highlight) ? 1 : 0.2,
            }}
          />
        ))}
      </div>
      <svg
        className="absolute inset-x-0 overflow-visible"
        style={{ top: above ? TAG_HEIGHT : BAR_HEIGHT, height: LEADER_HEIGHT }}
        viewBox={`0 0 ${extent} ${LEADER_HEIGHT}`}
        preserveAspectRatio="none"
        aria-hidden="true"
      >
        {entries.map((entry, index) => {
          const place = places[index];
          if (!place) {
            return null;
          }
          const centre = place.left + TAG_WIDTH / 2;
          // Drawn from whichever end the bar is on, so the dog-leg always leaves
          // the segment and arrives at the tag.
          const [from, at] = above
            ? [centre.toFixed(1), place.middle.toFixed(1)]
            : [place.middle.toFixed(1), centre.toFixed(1)];

          return (
            <path
              key={entry.label}
              d={`M${from} 0 V${LEADER_HEIGHT / 2} H${at} V${LEADER_HEIGHT}`}
              className="stroke-[var(--rule)]"
              strokeWidth={1}
              fill="none"
              // The viewBox is stretched to the measured width, so a plain
              // stroke would be scaled with it and land at some fraction of a
              // pixel.
              vectorEffect="non-scaling-stroke"
            />
          );
        })}
      </svg>
      <ul aria-label={classesLabel} className="contents">
        {entries.map((entry, index) => (
          <li
            key={entry.label}
            className="absolute"
            style={{
              left: places[index]?.left ?? 0,
              top: above ? 0 : BAR_HEIGHT + LEADER_HEIGHT,
              width: TAG_WIDTH,
              height: TAG_HEIGHT,
            }}
          >
            <HighlightToggle
              highlight={entry.highlight}
              current={highlight}
              onChange={onHighlightChange}
              // Both quantities spoken whichever one is drawn: the figure on
              // screen is a choice about space, not about what the class is.
              label={`${entry.label}, ${entry.description}, ${barLength(entry.metres, unitSystem, longest)}, ${formatShare(entry.share)} of the route`}
              title={entry.description}
              className="grid size-full content-center rounded px-0.5 text-center leading-tight hover:bg-[var(--base)] focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--accent)] aria-pressed:bg-[color-mix(in_srgb,var(--accent)_16%,transparent)]"
            >
              <span className="truncate text-[10px] text-[var(--ink-2)]">{entry.label}</span>
              <span className="truncate text-[11px] font-semibold tabular-nums">
                {barLength(entry.metres, unitSystem, longest)}
              </span>
            </HighlightToggle>
          </li>
        ))}
      </ul>
    </div>
  );
}
