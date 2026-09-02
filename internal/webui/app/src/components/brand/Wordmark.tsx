/**
 * The mark and the name, side by side.
 *
 * Only that. What the application is, said once, in the one place a reader
 * looks for it — the far left of the menu bar. Everything the row used to carry
 * besides the name is navigation, and navigation belongs to the bar rather than
 * to the brand standing at the end of it.
 */

import { cn } from "@/lib/utils";
import { Logo } from "./Logo";

export interface WordmarkProps {
  /** The mark's edge in pixels; the name is sized by `className`. */
  size?: number;
  className?: string;
}

export function Wordmark({ size = 22, className }: WordmarkProps) {
  return (
    <span className={cn("flex items-center gap-2 text-sm font-semibold tracking-tight", className)}>
      <Logo className="text-[var(--accent)]" size={size} />
      domestique
    </span>
  );
}
