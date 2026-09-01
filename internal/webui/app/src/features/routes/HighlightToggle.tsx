/**
 * One class of one mix, pressable.
 *
 * A plain click always reports its own class — pressed or not, what a repeat
 * press means is the highlight's owner's call, not this button's. Alt/cmd-click
 * is the one gesture this component owns outright: it reports `null`, the
 * whole route back. Only the appearance is otherwise the caller's business.
 */

import type { ReactNode } from "react";
import type { Highlight } from "../../lib/highlight";
import { sameHighlight } from "../../lib/highlight";

export function HighlightToggle({
  highlight,
  current,
  onChange,
  label,
  className,
  style,
  title,
  children,
}: {
  highlight: Highlight;
  current: Highlight | null;
  onChange: (next: Highlight | null) => void;
  /** Spoken instead of the contents, which are usually an abbreviation. */
  label: string;
  className?: string | undefined;
  style?: React.CSSProperties | undefined;
  title?: string | undefined;
  children: ReactNode;
}) {
  const pressed = sameHighlight(current, highlight);

  return (
    <button
      type="button"
      aria-pressed={pressed}
      aria-label={label}
      title={title}
      onClick={(event) => onChange(event.altKey || event.metaKey ? null : highlight)}
      className={`focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--accent)] ${className ?? ""}`}
      style={style}
    >
      {children}
    </button>
  );
}
