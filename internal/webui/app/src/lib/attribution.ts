/**
 * The credits this service owes for the data it shows, and how the tile ones
 * are read out of a style document.
 *
 * Every credit is shown in one place, the settings page's data sources card.
 * MapLibre's own attribution control stays switched off everywhere in this UI:
 * it renders the provider's own markup, which is markup from a third-party
 * origin.
 */

import { queryOptions } from "@tanstack/react-query";

/**
 * The surface classification is a derived OpenStreetMap database under the
 * ODbL, whose share-alike terms oblige this credit.
 */
export const SURFACE_ATTRIBUTION = "Surface data © OpenStreetMap contributors (ODbL)";

/** Open-Meteo's forecasts are CC BY 4.0, which obliges this credit. */
export const WEATHER_ATTRIBUTION = "Weather data by Open-Meteo.com";

/**
 * Reduces an attribution string to plain text.
 *
 * Parsing rather than a regex so entities such as `&copy;` decode properly.
 * `DOMParser` does not execute anything it parses, and only the text is read
 * back, so no third-party markup or script reaches the page.
 */
export function stripMarkup(value: string): string {
  const parsed = new DOMParser().parseFromString(value, "text/html");

  return (parsed.body.textContent ?? "").replace(/\s+/g, " ").trim();
}

async function readJSON(url: string): Promise<unknown> {
  const response = await fetch(url);

  return response.ok ? response.json() : null;
}

/**
 * Resolves a URL found inside a style document against that document.
 *
 * A style may reference its TileJSON relatively, and such a reference is
 * relative to the style, not to this page. Resolving it against the app origin
 * would request the wrong host whenever a configured basemap points at a
 * third-party provider. A value that will not parse is passed through
 * unchanged, so a malformed style degrades to no attribution rather than an
 * exception.
 */
function resolveAgainstStyle(styleUrl: string, url: string): string {
  try {
    return new URL(url, new URL(styleUrl, window.location.href)).toString();
  } catch {
    return url;
  }
}

function attributionOf(value: unknown): string {
  const attribution = (value as { attribution?: unknown } | null)?.attribution;

  return typeof attribution === "string" ? stripMarkup(attribution) : "";
}

/** Reads every credit one style document's sources declare. */
export async function fetchAttribution(styleUrl: string): Promise<string> {
  const style = await readJSON(styleUrl);
  const sources = (style as { sources?: Record<string, unknown> } | null)?.sources ?? {};

  const credits = new Set<string>();
  for (const source of Object.values(sources)) {
    const direct = attributionOf(source);
    if (direct !== "") {
      credits.add(direct);

      continue;
    }
    // A source may point at a TileJSON document instead of declaring its
    // attribution inline, which is how the default provider publishes it.
    const tileJSONURL = (source as { url?: unknown } | null)?.url;
    if (typeof tileJSONURL === "string") {
      const indirect = attributionOf(await readJSON(resolveAgainstStyle(styleUrl, tileJSONURL)));
      if (indirect !== "") {
        credits.add(indirect);
      }
    }
  }

  return [...credits].join(" · ");
}

/**
 * One basemap's credit, keyed on the style it is read from and kept for the
 * session: a style's attribution does not change under a reader.
 */
export function basemapAttributionQuery(styleUrl: string) {
  return queryOptions({
    queryKey: ["tile-attribution", styleUrl] as const,
    queryFn: () => fetchAttribution(styleUrl),
    staleTime: Number.POSITIVE_INFINITY,
  });
}
