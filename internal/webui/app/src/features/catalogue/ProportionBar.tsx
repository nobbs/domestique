/**
 * A route's division into classes as one bar sized by share: width itself
 * reads as proportion. Labels sit under the segments, never on the colour.
 */

import { formatShare } from "../../lib/mix";

/** Below this share, a segment carries no label: not enough width for one. */
const LABEL_THRESHOLD = 0.08;

export interface ProportionSegment {
  key: string;
  label: string;
  /** A CSS colour, normally one of the palette's custom properties. */
  colour: string;
  /** Of the whole route, from 0 to 1. */
  share: number;
}

export function ProportionBar({
  segments,
  description,
}: {
  segments: readonly ProportionSegment[];
  /** What this bar divides, read before the per-segment shares: "Surface", "Gradient". */
  description: string;
}) {
  const spoken = segments.map((segment) => `${segment.label} ${formatShare(segment.share)}`);

  return (
    <div
      className="flex w-full items-start gap-px"
      role="img"
      aria-label={`${description}: ${spoken.join(", ")}`}
    >
      {segments.map((segment, index) => (
        <div
          key={segment.key}
          title={spoken[index]}
          style={{ flexGrow: segment.share, flexBasis: 0 }}
          className="flex min-w-0 flex-col items-center gap-1"
        >
          <span className="block h-2 w-full rounded-[2px]" style={{ background: segment.colour }} />
          {segment.share < LABEL_THRESHOLD ? null : (
            <span className="w-full truncate text-center text-[10px] text-[var(--ink-2)] leading-none tabular-nums">
              {spoken[index]}
            </span>
          )}
        </div>
      ))}
    </div>
  );
}
