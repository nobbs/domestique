/**
 * Credits the tile source once for the whole library grid.
 *
 * The cards drop MapLibre's own attribution control, because one per card would
 * be unreadable, but the tile and map data licences still require visible
 * credit. Reading it out of the style document keeps it correct if the operator
 * points `webui.tile_style_url` at a different provider.
 *
 * The text is stripped of markup rather than rendered as HTML: the style comes
 * from a third-party origin, and no third-party markup is injected into this
 * page.
 */

import { useQuery } from "@tanstack/react-query";
import { webUIConfigQuery } from "../../api/queries";

/**
 * Reduces an attribution string to plain text.
 *
 * Parsing rather than a regex so entities such as `&copy;` decode properly.
 * `DOMParser` does not execute anything it parses, and only the text is read
 * back, so no third-party markup or script reaches the page.
 */
function stripMarkup(value: string): string {
  const parsed = new DOMParser().parseFromString(value, "text/html");

  return (parsed.body.textContent ?? "").replace(/\s+/g, " ").trim();
}

async function readJSON(url: string): Promise<unknown> {
  const response = await fetch(url);

  return response.ok ? response.json() : null;
}

function attributionOf(value: unknown): string {
  const attribution = (value as { attribution?: unknown } | null)?.attribution;

  return typeof attribution === "string" ? stripMarkup(attribution) : "";
}

async function fetchAttribution(styleUrl: string): Promise<string> {
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
      const indirect = attributionOf(await readJSON(tileJSONURL));
      if (indirect !== "") {
        credits.add(indirect);
      }
    }
  }

  return [...credits].join(" · ");
}

export function MapAttribution() {
  const config = useQuery(webUIConfigQuery());
  const styleUrl = config.data?.tileStyleUrl;

  const attribution = useQuery({
    queryKey: ["tile-attribution", styleUrl] as const,
    queryFn: () => fetchAttribution(styleUrl ?? ""),
    enabled: styleUrl !== undefined,
    staleTime: Number.POSITIVE_INFINITY,
  });

  if (!attribution.data) {
    return null;
  }

  return <footer className="grid-attribution">{attribution.data}</footer>;
}
