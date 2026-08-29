/**
 * The credits this service owes. MapLibre's own attribution control stays off
 * everywhere: it renders markup from a third-party origin.
 */

import { queryOptions } from "@tanstack/react-query";

/** The surface classification is a derived OpenStreetMap database under the ODbL. */
export const SURFACE_ATTRIBUTION = "Surface data © OpenStreetMap contributors (ODbL)";

/** Open-Meteo's forecasts are CC BY 4.0. */
export const WEATHER_ATTRIBUTION = "Weather data by Open-Meteo.com";

/**
 * Reduces an attribution string to plain text. Parsed rather than matched so
 * `&copy;` decodes; only the text is read back, so no markup reaches the page.
 */
export function stripMarkup(value: string): string {
  const parsed = new DOMParser().parseFromString(value, "text/html");

  return (parsed.body.textContent ?? "").replace(/\s+/g, " ").trim();
}

/**
 * Reads one JSON document, or null for anything that is not one. Nothing here
 * rejects: an unreachable provider costs its own credit, not the whole card.
 */
async function readJSON(url: string): Promise<unknown> {
  try {
    const response = await fetch(url);

    return response.ok ? await response.json() : null;
  } catch {
    return null;
  }
}

/**
 * Resolves a URL found inside a style document against that document: a
 * relative TileJSON reference belongs to the style's origin, not this page's.
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
export async function fetchAttribution(styleUrl: string): Promise<string[]> {
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

  return [...credits];
}

/**
 * One basemap's credits, from every style it may load. A dark twin usually
 * repeats its entry's credit and dedupes away; one that does not is a credit
 * this page owes.
 */
export function basemapAttributionQuery(styleUrl: string, styleUrlDark?: string | undefined) {
  return queryOptions({
    queryKey: ["tile-attribution", styleUrl, styleUrlDark ?? ""] as const,
    queryFn: async () => {
      const styles = styleUrlDark === undefined ? [styleUrl] : [styleUrl, styleUrlDark];
      const credits = await Promise.all(styles.map(fetchAttribution));

      return [...new Set(credits.flat())];
    },
    staleTime: Number.POSITIVE_INFINITY,
  });
}
