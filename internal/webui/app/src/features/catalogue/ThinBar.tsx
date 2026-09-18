/**
 * A thin split bar with square joins and a hairline gap, as Volume draws its
 * grounds: a route's division into classes, at a size that fits under a name.
 */

import { formatShare } from "../../lib/mix";

export interface ThinBarSegment {
  key: string;
  label: string;
  /** A CSS colour, normally one of the palette's custom properties. */
  colour: string;
  /** Of the whole route, from 0 to 1. */
  share: number;
}

export function ThinBar({
  segments,
  label,
}: {
  segments: readonly ThinBarSegment[];
  /** What this bar divides, read before the per-segment shares: "Surface", "Gradient". */
  label: string;
}) {
  return (
    <span
      role="img"
      aria-label={`${label}: ${segments.map((segment) => `${segment.label} ${formatShare(segment.share)}`).join(", ")}`}
      className="flex h-1.5 w-full gap-0.5 overflow-hidden rounded-full"
    >
      {segments.map((segment) => (
        <span
          key={segment.key}
          title={`${segment.label} ${formatShare(segment.share)}`}
          style={{ flexGrow: segment.share, flexBasis: 0, background: segment.colour }}
        />
      ))}
    </span>
  );
}
