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

import { useSyncExternalStore } from "react";
import type { WebUIConfig } from "../api/types";

const DARK_SCHEME = "(prefers-color-scheme: dark)";

/**
 * Subscribes to the system colour scheme.
 *
 * A fresh `MediaQueryList` per subscription rather than one held at module
 * scope: matching is stateless, the browser deduplicates the underlying
 * listener, and a module-level query would evaluate at import time, before any
 * test has had the chance to stub `matchMedia`.
 */
function subscribe(onChange: () => void): () => void {
  const query = window.matchMedia(DARK_SCHEME);
  query.addEventListener("change", onChange);

  return () => {
    query.removeEventListener("change", onChange);
  };
}

function prefersDark(): boolean {
  return window.matchMedia(DARK_SCHEME).matches;
}

/**
 * Reports whether the system asks for a dark colour scheme, and re-renders when
 * that changes.
 *
 * `useSyncExternalStore` rather than an effect that seeds state: an effect runs
 * after the first paint, so a dark-mode page would build its map on the light
 * style and immediately swap it — a visible flash and a wasted style fetch.
 */
export function usePrefersDarkScheme(): boolean {
  return useSyncExternalStore(subscribe, prefersDark);
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
