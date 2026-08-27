/**
 * The mark and the name, side by side.
 *
 * Only that. What the application is, said once, in the one place a reader
 * looks for it — the far left of the menu bar. Everything the row used to carry
 * besides the name is navigation, and navigation belongs to the bar rather than
 * to the brand standing at the end of it.
 */

import { Logo } from "./Logo";

export function Wordmark() {
  return (
    <span className="flex items-center gap-2">
      <Logo className="text-[var(--accent)]" size={22} />
      <span className="text-sm font-semibold tracking-tight">domestique</span>
    </span>
  );
}
