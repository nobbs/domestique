/**
 * Which basemap style the map loads, following the system colour scheme.
 *
 * The rest of the UI switches its own palette in CSS, at the `prefers-color-scheme`
 * query in `index.css`. The basemap cannot: MapLibre paints on a canvas from a
 * style document fetched over the network, so the choice has to be made in
 * JavaScript and the document swapped. This module is that decision, kept in one
 * place so every map in the application makes it identically.
 *
 * The operator may configure no dark style — a provider that publishes only one
 * — in which case that one style is used in both schemes, which is what every
 * deployment did before this existed.
 */

import type { WebUIConfig } from "../api/types";
import { useMediaQuery } from "./mediaQuery";

const DARK_SCHEME = "(prefers-color-scheme: dark)";

/**
 * Reports whether the system asks for a dark colour scheme, and re-renders when
 * that changes. See `mediaQuery.ts` for why it is not an effect.
 */
export function usePrefersDarkScheme(): boolean {
  return useMediaQuery(DARK_SCHEME);
}

/** The basemap on screen, and what it looks like. */
export interface Basemap {
  /** The style document to load. */
  styleUrl: string;

  /**
   * Whether that document is the dark one.
   *
   * Not the same question as `usePrefersDarkScheme`, and the difference is the
   * reason this is reported rather than inferred: with no dark style configured
   * the light cartography stays on screen under a dark system scheme. Anything
   * painted over the map has to match the ground actually loaded, not the scheme
   * the system asked for.
   */
  dark: boolean;
}

/** The basemap for the scheme in force. */
export function basemapFor(config: WebUIConfig, prefersDark: boolean): Basemap {
  const darkStyleUrl = config.tileStyleUrlDark;
  if (prefersDark && darkStyleUrl) {
    return { styleUrl: darkStyleUrl, dark: true };
  }

  return { styleUrl: config.tileStyleUrl, dark: false };
}
