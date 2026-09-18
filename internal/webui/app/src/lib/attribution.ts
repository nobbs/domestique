/**
 * The credits this service owes. MapLibre's own attribution control stays off
 * everywhere: it renders markup from a third-party origin.
 */

import { queryOptions } from "@tanstack/react-query";

/** One credit as it is shown: its words, and where they lead when the provider says. */
export interface Credit {
  text: string;
  href?: string;
}

const OSM: Credit = {
  text: "© OpenStreetMap contributors",
  href: "https://www.openstreetmap.org/copyright",
};

/** The surface classification is a derived OpenStreetMap database under the ODbL. */
export const SURFACE_CREDITS: Credit[] = [
  OSM,
  { text: "ODbL", href: "https://opendatacommons.org/licenses/odbl/" },
];

/** Open-Meteo's forecasts are CC BY 4.0. */
export const WEATHER_CREDITS: Credit[] = [
  { text: "Open-Meteo.com", href: "https://open-meteo.com/" },
  { text: "CC BY 4.0", href: "https://creativecommons.org/licenses/by/4.0/" },
];

/** The planner's engine, its gravel profile's source, and the ground both route over. */
export const ROUTING_CREDITS: Credit[] = [
  { text: "BRouter", href: "https://brouter.de/brouter/" },
  { text: "Bikerouter", href: "https://bikerouter.de/" },
  OSM,
];

const clean = (value: string | null) => (value ?? "").replace(/\s+/g, " ").trim();

/**
 * Reads an attribution string as credits: one per link, or its whole text when
 * it links nothing. Only text and an http(s) href are read back, never markup.
 */
export function parseCredits(value: string): Credit[] {
  const body = new DOMParser().parseFromString(value, "text/html").body;
  const links = [...body.querySelectorAll("a")].flatMap((anchor) => {
    const text = clean(anchor.textContent);
    const href = anchor.getAttribute("href") ?? "";
    if (text === "") {
      return [];
    }
    return /^https?:\/\//.test(href) ? [{ text, href }] : [{ text }];
  });
  const text = clean(body.textContent);

  return links.length > 0 ? links : text === "" ? [] : [{ text }];
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

function attributionOf(value: unknown): Credit[] {
  const attribution = (value as { attribution?: unknown } | null)?.attribution;

  return typeof attribution === "string" ? parseCredits(attribution) : [];
}

/** Drops repeats: credits leading to one place count once, under the first words given. */
export function uniqueCredits(credits: Credit[]): Credit[] {
  const seen = new Map<string, Credit>();
  for (const credit of credits) {
    const key = credit.href ?? credit.text;
    if (!seen.has(key)) {
      seen.set(key, credit);
    }
  }
  return [...seen.values()];
}

/** Reads every credit one style document's sources declare. */
export async function fetchAttribution(styleUrl: string): Promise<Credit[]> {
  const style = await readJSON(styleUrl);
  const sources = (style as { sources?: Record<string, unknown> } | null)?.sources ?? {};

  const credits: Credit[] = [];
  for (const source of Object.values(sources)) {
    const direct = attributionOf(source);
    if (direct.length > 0) {
      credits.push(...direct);

      continue;
    }
    // A source may point at a TileJSON document instead of declaring its
    // attribution inline, which is how the default provider publishes it.
    const tileJSONURL = (source as { url?: unknown } | null)?.url;
    if (typeof tileJSONURL === "string") {
      credits.push(...attributionOf(await readJSON(resolveAgainstStyle(styleUrl, tileJSONURL))));
    }
  }

  return uniqueCredits(credits);
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

      return uniqueCredits(credits.flat());
    },
    staleTime: Number.POSITIVE_INFINITY,
  });
}
