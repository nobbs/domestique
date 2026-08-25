/**
 * The tile and map-data credit, in the map's own control cluster.
 *
 * MapLibre's attribution control is switched off everywhere in this UI: it puts
 * a fourth thing in a fourth corner, and it renders the provider's own markup.
 * The tile and map data licences still require visible credit, so the text is
 * read out of the style document instead, which keeps it correct whichever of
 * the configured basemaps is on screen — the query below is keyed on the style
 * URL, so switching provider refetches that provider's own attribution.
 *
 * The text is stripped of markup rather than rendered as HTML: the style comes
 * from a third-party origin, and no third-party markup is injected into this
 * page.
 *
 * It folds behind a button rather than competing with the map. It cannot simply
 * go away — the licences oblige the credit to be reachable — and the existing
 * labelled control reveals it in one interaction. The reader's own choice wins
 * from then on.
 *
 */

import { IconInfoCircle } from "@tabler/icons-react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

/** What the button expands, named so the button can point at it. */
const CREDIT_TEXT_ID = "map-credit-text";

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
      const indirect = attributionOf(await readJSON(resolveAgainstStyle(styleUrl, tileJSONURL)));
      if (indirect !== "") {
        credits.add(indirect);
      }
    }
  }

  return [...credits].join(" · ");
}

export interface MapCreditsProps {
  /** The style document the credit is read out of. */
  styleUrl: string | undefined;
  /**
   * A credit the style cannot know about, joined to the ones it declares.
   *
   * The surface classification is a separate derived database under the ODbL,
   * whose share-alike terms oblige attribution wherever it is shown.
   */
  extra?: string | undefined;
  /**
   * Whether the reader folded the credit open or away, or `null` while they
   * have said nothing and it begins folded.
   *
   * Three states rather than two, because "not yet asked" is not the same
   * answer as "asked for it folded".
   */
  choice: boolean | null;
  onChoiceChange: (choice: boolean) => void;
}

export function MapCredits({ styleUrl, extra, choice, onChoiceChange }: MapCreditsProps) {
  const attribution = useQuery({
    queryKey: ["tile-attribution", styleUrl] as const,
    queryFn: () => fetchAttribution(styleUrl ?? ""),
    enabled: styleUrl !== undefined,
    staleTime: Number.POSITIVE_INFINITY,
  });
  const expanded = choice ?? false;

  const credits = [attribution.data, extra].filter(Boolean).join(" · ");
  if (credits === "") {
    return null;
  }

  return (
    <div
      className={cn(
        "map-credits z-[1] flex max-w-[min(100%,380px)] items-center gap-1 pointer-events-auto",
        expanded && "rounded-md border border-border bg-background p-1 shadow-sm",
      )}
    >
      <Button
        variant={expanded ? "ghost" : "outline"}
        size="icon-xs"
        type="button"
        aria-expanded={expanded}
        // The mark says "there is something to read here" to anyone who can see
        // it; the name says what, for anyone who cannot. `aria-expanded` is what
        // reports the state — the glyph does not change and must not be the only
        // thing carrying it.
        aria-label={expanded ? "Hide the map credit" : "Show the map credit"}
        // Only while there is text to point at, because the credit is unmounted
        // rather than hidden when folded and a control naming an element outside
        // the document is a reference a screen reader cannot follow.
        {...(expanded ? { "aria-controls": CREDIT_TEXT_ID } : {})}
        onClick={() => onChoiceChange(!expanded)}
      >
        <IconInfoCircle data-icon="inline-start" stroke={1.2} aria-hidden="true" />
      </Button>
      {expanded ? (
        <p className="text-xs text-muted-foreground" id={CREDIT_TEXT_ID}>
          {credits}
        </p>
      ) : null}
    </div>
  );
}
