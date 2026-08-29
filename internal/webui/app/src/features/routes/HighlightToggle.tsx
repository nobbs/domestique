/**
 * One class of one mix, pressable.
 *
 * Several things offer the reader a class to pick out — an upright bar's
 * labels, a ribbon's own class names, a chip — and all of them have to agree
 * that pressing the pressed one gives the whole route back. Only the appearance
 * is the caller's business, so this carries none.
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
      onClick={() => onChange(pressed ? null : highlight)}
      className={`focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--accent)] ${className ?? ""}`}
      style={style}
    >
      {children}
    </button>
  );
}
