/**
 * The classes present in one mix, as the press targets.
 *
 * Lifted from the terrain card in the panel spike rather than imported from
 * it: that one is a component of an alternative still being compared, and a
 * shared control would mean editing one spike to change the other. The
 * palette and the entries underneath it are shared; this much markup is not.
 */

import type { Highlight } from "../../../lib/highlight";
import type { MixEntry } from "../panel-spike/shared";
import { formatShare, HighlightToggle } from "../panel-spike/shared";

export function ClassList({
  name,
  entries,
  absence,
  highlight,
  onHighlightChange,
}: {
  name: string;
  entries: MixEntry[];
  absence: string | null;
  highlight: Highlight | null;
  onHighlightChange: (next: Highlight | null) => void;
}) {
  if (entries.length === 0) {
    return <p className="text-xs text-[var(--ink-2)]">{absence}</p>;
  }

  return (
    <ul className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
      <li className="text-[11px] font-semibold tracking-[0.06em] text-[var(--ink-2)] uppercase">
        {name}
      </li>
      {entries.map((entry) => (
        <li key={entry.label}>
          <HighlightToggle
            highlight={entry.highlight}
            current={highlight}
            onChange={onHighlightChange}
            label={`${entry.label}, ${entry.description}, ${formatShare(entry.share)} of the route`}
            title={entry.description}
            className="flex items-center gap-1 rounded px-1 py-0.5 text-xs hover:bg-[var(--base)] aria-pressed:bg-[color-mix(in_srgb,var(--accent)_14%,transparent)]"
          >
            <span
              className="size-2 shrink-0 rounded-[2px]"
              style={{ background: entry.colour }}
              aria-hidden="true"
            />
            <span className="text-[var(--ink-2)]">{entry.label}</span>
            <span className="font-semibold tabular-nums">{formatShare(entry.share)}</span>
          </HighlightToggle>
        </li>
      ))}
    </ul>
  );
}
