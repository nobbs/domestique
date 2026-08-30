/**
 * Which cartography the map's children are drawn on.
 *
 * Whether the loaded basemap is dark is a fact about the canvas everything on
 * it shares, not a choice any one layer makes — a deployment with no dark
 * style configured keeps the light basemap under a dark page, so this is
 * resolved where the basemap itself is (see `basemapFor`) and carried here
 * rather than threaded as a prop through every component in between.
 */

import { createContext, type ReactNode, useContext, useMemo } from "react";

export interface Cartography {
  /** Whether the loaded basemap is the dark one — see `LoadedBasemap.dark`. */
  dark: boolean;
}

const CartographyContext = createContext<Cartography | null>(null);

export function CartographyProvider({ dark, children }: Cartography & { children: ReactNode }) {
  const value = useMemo(() => ({ dark }), [dark]);

  return <CartographyContext.Provider value={value}>{children}</CartographyContext.Provider>;
}

/**
 * The cartography this component is drawn on.
 *
 * Throws rather than defaulting: a layer rendered outside a provider would
 * otherwise pick its colours for a basemap nobody resolved, and mis-render
 * quietly instead of failing where the mistake is.
 */
export function useCartography(): Cartography {
  const cartography = useContext(CartographyContext);
  if (cartography === null) {
    throw new Error("useCartography must be used under a CartographyProvider");
  }

  return cartography;
}
