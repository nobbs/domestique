/**
 * A mix as a bar across rather than a bar up.
 *
 * The column was the shape the old card had room for: two of them side by
 * side, each about a hundred and sixty pixels wide, with the labels stacked in
 * whatever height was left. Across, one mix gets the whole card — so the
 * segments are drawn at the width they actually differ by, and the classes sit
 * under the ground they name instead of beside it.
 *
 * The tag-and-pointer idea is unchanged, and so is the placement: each tag
 * wants to sit under the middle of its own segment, and where two would
 * overlap the later one is pushed along and a leader keeps the correspondence
 * explicit. It is the column's own algorithm with the axes swapped — extent is
 * the bar's width rather than its height, and the fixed size is the tag's
 * width rather than the label's height.
 */

import { HighlightToggle } from "../../../components/route/HighlightToggle";
import { formatDistance } from "../../../lib/format";
import type { Highlight } from "../../../lib/highlight";
import type { MixEntry } from "../../../lib/mix";
import { useElementWidth } from "../../../lib/useElementWidth";

const BAR_HEIGHT = 10;
/** The drop the leader falls through, between the bar and the tags. */
const LEADER_HEIGHT = 12;
/**
 * One tag's slot.
 *
 * Fixed, for the reason the column's row height is: the placement needs a size
 * it can reserve before it knows what the text comes to, and a slot measured
 * per tag would let a long class name shove its neighbours around. Wide enough
 * for `Compacted` over `10.2 km` at these sizes.
 */
const TAG_WIDTH = 62;
const TAG_HEIGHT = 26;

/** The card's own content width, until the container has been measured. */
const ASSUMED_WIDTH = 344;

/**
 * Where each tag goes, given where its segment is — the column's placement,
 * turned on its side.
 */
function placeTags(shares: number[], extent: number): { middle: number; left: number }[] {
  let offset = 0;
  const middles = shares.map((share) => {
    const middle = offset + (share * extent) / 2;
    offset += share * extent;

    return middle;
  });

  const lefts: number[] = [];
  let rightmost = 0;
  for (const middle of middles) {
    const left = Math.max(middle - TAG_WIDTH / 2, rightmost);
    lefts.push(left);
    rightmost = left + TAG_WIDTH;
  }

  // The forward pass can push the last tag off the end. Walking back from the
  // right edge gives every tag the earliest place it can hold without landing
  // on the one after it.
  if (rightmost > extent) {
    let wall = extent;
    for (let index = lefts.length - 1; index >= 0; index--) {
      const left = Math.max(Math.min(lefts[index] ?? 0, wall - TAG_WIDTH), 0);
      lefts[index] = left;
      wall = left;
    }
  }

  return middles.map((middle, index) => ({ middle, left: lefts[index] ?? middle }));
}

export function MixRow({
  name,
  classesLabel,
  entries,
  absence,
  tagSide = "below",
  gapped = false,
  highlight,
  onHighlightChange,
}: {
  name: string;
  classesLabel: string;
  entries: MixEntry[];
  absence: string;
  /**
   * Which side of the bar the tags hang off.
   *
   * The pair is drawn mirrored — gradient's tags above its bar, surface's
   * below its own — so the two bars meet in the middle with nothing between
   * them and the labelling fans outward. It puts the two things being compared
   * a few pixels apart instead of a caption and a heading apart.
   */
  tagSide?: "above" | "below";
  /**
   * Whether the segments are separated rather than butted together.
   *
   * The reason this is a question at all: the dock's ground ribbon is a
   * horizontal bar of the same classes in the same colours, and it is
   * *positional* — it says where the gravel is. This one is proportional and
   * sorted by size, so ridden order is exactly what it does not show. Butted
   * together the two are the same picture meaning two different things, four
   * hundred pixels apart. Gaps make this one read as a chart of amounts.
   */
  gapped?: boolean;
  highlight: Highlight | null;
  onHighlightChange: (next: Highlight | null) => void;
}) {
  const above = tagSide === "above";
  const { ref, width } = useElementWidth<HTMLDivElement>();
  const extent = width > 0 ? width : ASSUMED_WIDTH;
  const places = placeTags(
    entries.map((entry) => entry.share),
    extent,
  );

  return (
    <section aria-label={name}>
      {entries.length === 0 ? (
        <p className="text-xs text-[var(--ink-2)]">{absence}</p>
      ) : (
        <div
          ref={ref}
          className="relative"
          style={{ height: BAR_HEIGHT + LEADER_HEIGHT + TAG_HEIGHT }}
        >
          <div
            className={`absolute inset-x-0 flex ${gapped ? "gap-0.5" : "overflow-hidden rounded-sm"}`}
            style={{ height: BAR_HEIGHT, ...(above ? { bottom: 0 } : { top: 0 }) }}
            aria-hidden="true"
          >
            {entries.map((entry) => (
              <span
                key={entry.label}
                className={gapped ? "rounded-xs" : ""}
                style={{
                  flexGrow: entry.share,
                  flexBasis: 0,
                  background: entry.colour,
                  opacity:
                    highlight === null ||
                    (highlight.type === entry.highlight.type &&
                      JSON.stringify(highlight) === JSON.stringify(entry.highlight))
                      ? 1
                      : 0.2,
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
              const to = place.left + TAG_WIDTH / 2;
              // Drawn from whichever end the bar is on, so the dog-leg always
              // leaves the segment and arrives at the tag.
              const [from, at] = above
                ? [to.toFixed(1), place.middle.toFixed(1)]
                : [place.middle.toFixed(1), to.toFixed(1)];

              return (
                <path
                  key={entry.label}
                  d={`M${from} 0 V${LEADER_HEIGHT / 2} H${at} V${LEADER_HEIGHT}`}
                  className="stroke-[var(--rule)]"
                  strokeWidth={1}
                  fill="none"
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
                  label={`${entry.label}, ${entry.description}, ${formatDistance(entry.metres, "metric")}`}
                  title={entry.description}
                  className="grid size-full content-center rounded px-0.5 text-center leading-tight hover:bg-[var(--base)] aria-pressed:bg-[color-mix(in_srgb,var(--accent)_16%,transparent)]"
                >
                  <span className="truncate text-[10px] text-[var(--ink-2)]">{entry.label}</span>
                  <span className="truncate text-[11px] font-semibold tabular-nums">
                    {formatDistance(entry.metres, "metric")}
                  </span>
                </HighlightToggle>
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}
