/**
 * Which basemap style the map loads: the reader's pick, in the system's scheme.
 *
 * The rest of the UI switches its own palette in CSS, at the `prefers-color-scheme`
 * query in `index.css`. The basemap cannot: MapLibre paints on a canvas from a
 * style document fetched over the network, so the choice has to be made in
 * JavaScript and the document swapped. This module is that decision, kept in one
 * place so every map in the application makes it identically.
 *
 * Two answers are folded together here. The operator names the basemaps on
 * offer, and the reader picks one of them; then the scheme in force picks
 * between that entry's own light and dark styles, if it published both. An
 * operator may configure no dark style — a provider that publishes only one —
 * in which case that one style is used in both schemes.
 */

import { useCallback, useState } from "react";
import type { Basemap, WebUIConfig } from "../api/types";
import { useMediaQuery } from "./mediaQuery";

const DARK_SCHEME = "(prefers-color-scheme: dark)";

/**
 * Where the reader's pick is remembered.
 *
 * Namespaced because the origin may well be shared with whatever else the
 * operator runs on the same host. It holds a basemap name and nothing else: not
 * a style URL, not a key, and nothing about the library.
 */
const CHOICE_STORAGE_KEY = "domestique.basemap";

/**
 * Reports whether the system asks for a dark colour scheme, and re-renders when
 * that changes. See `mediaQuery.ts` for why it is not an effect.
 */
export function usePrefersDarkScheme(): boolean {
  return useMediaQuery(DARK_SCHEME);
}

/** The basemap on screen, and what it looks like. */
export interface LoadedBasemap {
  /**
   * Which of the configured basemaps this is.
   *
   * Reported rather than left to the caller because the fallbacks below decide
   * it: a reader who has picked nothing, or picked something the operator has
   * since removed, is looking at an entry they did not name. The picker marks
   * whichever entry this says, so what is checked is always the ground actually
   * loaded.
   */
  name: string;

  /** The style document to load. */
  styleUrl: string;

  /**
   * Whether the ground that document paints is dark.
   *
   * Not the same question as `usePrefersDarkScheme`, and the difference is the
   * reason this is reported rather than inferred. It is true two ways: a dark
   * style loaded because the system asked for one, or a cartography the operator
   * marked dark in either scheme, which is what satellite imagery is. It is
   * false where a provider publishes no dark style and its light cartography
   * therefore stays on screen after dark. Anything painted over the map has to
   * match the ground actually loaded, not the scheme the system asked for.
   */
  dark: boolean;
}

/**
 * The basemap the reader picked, in the scheme in force.
 *
 * A name that is not on offer falls back to the first entry rather than to
 * nothing: the name may have been remembered from before the operator edited
 * the list, and an edit to a config file must not leave a returning reader
 * looking at an empty map.
 */
export function basemapFor(
  config: WebUIConfig,
  prefersDark: boolean,
  selectedName?: string | null,
): LoadedBasemap {
  const entry = entryFor(config, selectedName);
  // The service refuses an empty list at startup and the parser refuses one on
  // the wire, so there is always an entry. The fallback is a total function
  // rather than a claim about the data.
  if (entry === undefined) {
    return { name: "", styleUrl: "", dark: false };
  }
  if (entry.darkCartography) {
    return { name: entry.name, styleUrl: entry.styleUrl, dark: true };
  }

  const darkStyleUrl = entry.styleUrlDark;
  if (prefersDark && darkStyleUrl) {
    return { name: entry.name, styleUrl: darkStyleUrl, dark: true };
  }

  return { name: entry.name, styleUrl: entry.styleUrl, dark: false };
}

function entryFor(config: WebUIConfig, selectedName?: string | null): Basemap | undefined {
  const picked = selectedName
    ? config.basemaps.find((basemap) => basemap.name === selectedName)
    : undefined;

  return picked ?? config.basemaps[0];
}

/**
 * The name of the basemap the reader picked, remembered across visits.
 *
 * Kept out of the address, unlike the open route: which ground a map is drawn
 * on is how one reader likes to look at the library rather than something worth
 * sending to somebody else, and a link that also changed the recipient's
 * basemap would be saying more than it meant to.
 *
 * Every touch of storage is guarded, because a browser may refuse it outright —
 * private windows and blocked third-party storage both throw on access rather
 * than returning nothing. A refusal costs the choice its memory, not the page
 * its map.
 */
export function useBasemapChoice(): [string | null, (name: string) => void] {
  const [choice, setChoice] = useState<string | null>(readChoice);

  const choose = useCallback((name: string) => {
    setChoice(name);
    writeChoice(name);
  }, []);

  return [choice, choose];
}

function readChoice(): string | null {
  try {
    return window.localStorage.getItem(CHOICE_STORAGE_KEY);
  } catch {
    return null;
  }
}

function writeChoice(name: string): void {
  try {
    window.localStorage.setItem(CHOICE_STORAGE_KEY, name);
  } catch {
    // Remembering is the whole of what is lost, and the pick still stands for
    // as long as the page is open.
  }
}
