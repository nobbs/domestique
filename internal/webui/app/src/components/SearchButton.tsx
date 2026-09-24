/** The menu bar's way into the search palette, which `App` mounts once for every page. */

import { IconSearch } from "@tabler/icons-react";
import { useSearchPalette } from "../lib/searchPalette";

export function SearchButton() {
  const { setOpen } = useSearchPalette();

  return (
    <button
      type="button"
      aria-label="Search"
      onClick={() => setOpen(true)}
      className="inline-flex h-8 shrink-0 items-center gap-2 rounded-[9px] bg-[var(--muted)] px-2.5 text-[var(--ink-2)] text-sm hover:text-[var(--ink)]"
    >
      <IconSearch size={15} stroke={1.8} aria-hidden="true" />
      <span className="hidden sm:inline">Search</span>
      <kbd className="hidden rounded-[6px] bg-[var(--panel)] px-1.5 font-sans text-xs sm:inline">
        ⌘K
      </kbd>
    </button>
  );
}
