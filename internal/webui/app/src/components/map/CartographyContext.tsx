/**
 * Which cartography the map's children are drawn on: the loaded basemap's own
 * darkness (resolved by `basemapFor`), not the page's scheme — a deployment
 * with no dark style keeps the light basemap under a dark page.
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
 * The cartography this component is drawn on. Throws outside a provider: a
 * default would colour for a basemap nobody resolved and mis-render quietly.
 */
export function useCartography(): Cartography {
  const cartography = useContext(CartographyContext);
  if (cartography === null) {
    throw new Error("useCartography must be used under a CartographyProvider");
  }

  return cartography;
}
