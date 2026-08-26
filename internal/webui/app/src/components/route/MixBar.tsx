import type { ReactNode } from "react";

/**
 * One proportion bar.
 *
 * Hidden from assistive technology: every segment in it is a chip below it with
 * its own figure spoken in full, and a bar is a picture of figures that have
 * already been said.
 */
export function MixBar({ children }: { children: ReactNode }) {
  return (
    <span className="mb-1 flex h-1.5 w-full overflow-hidden rounded-sm" aria-hidden="true">
      {children}
    </span>
  );
}
