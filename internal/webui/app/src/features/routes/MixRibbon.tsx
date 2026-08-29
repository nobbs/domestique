/**
 * A mix drawn in the order it is ridden.
 *
 * The proportion bar says *how much* of the route is gravel; this says *where*
 * the gravel is, which is a different question and the one a chart's axis can
 * answer. `gradientMix` and `gradientShares` exist as separate functions for
 * exactly this reason.
 *
 * Hidden from assistive technology, like the proportion bar it stands beside.
 * Every class in it is named with its share by whatever labels it, and a
 * picture of an order cannot be read out as one anyway.
 */

import type { Highlight } from "../../lib/highlight";
import { sameHighlight } from "../../lib/highlight";
import type { Segment } from "../../lib/mix";

export function MixRibbon({
  segments,
  className,
  highlight,
}: {
  segments: Segment[];
  /** The ribbon's own height, which is the only thing that varies by caller. */
  className: string;
  highlight: Highlight | null;
}) {
  return (
    <div className={`flex w-full overflow-hidden rounded-[3px] ${className}`} aria-hidden="true">
      {segments.map((segment) => (
        <div
          key={segment.key}
          style={{
            flexGrow: segment.share,
            flexBasis: 0,
            background: segment.colour,
            // Picking a class fades everything that is not it, which is the
            // answer the map gives to the same press. A ribbon is a map of the
            // route by distance, so it fades the same way.
            opacity: highlight === null || sameHighlight(highlight, segment.highlight) ? 1 : 0.16,
          }}
        />
      ))}
    </div>
  );
}
