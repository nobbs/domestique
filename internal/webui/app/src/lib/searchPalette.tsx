/**
 * Whether the search palette is open, shared between the menu bar's button and
 * the palette `App` mounts once: the bar is presentational and cannot import
 * the feature that draws the palette, so both meet here.
 */

import { createContext, type ReactNode, useContext, useMemo, useState } from "react";

interface SearchPaletteState {
  open: boolean;
  setOpen: (open: boolean | ((current: boolean) => boolean)) => void;
}

const SearchPaletteContext = createContext<SearchPaletteState>({
  open: false,
  setOpen: () => {},
});

export function SearchPaletteProvider({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false);
  const value = useMemo(() => ({ open, setOpen }), [open]);

  return <SearchPaletteContext.Provider value={value}>{children}</SearchPaletteContext.Provider>;
}

/** The palette's open state; outside a provider, a closed palette that cannot open. */
export function useSearchPalette(): SearchPaletteState {
  return useContext(SearchPaletteContext);
}
