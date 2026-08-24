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
 * It folds behind a button rather than always standing open, because on a phone
 * the line is a strip of small text across the whole map. It cannot simply go
 * away — the licences oblige the credit to be visible — but attribution
 * guidance accepts a credit collapsed behind an affordance on a constrained
 * display, provided it is there and one interaction away, which is the same
 * bargain MapLibre's own `compact` attribution strikes. So the fold follows the
 * room available: open where there is space for the line, away where there is
 * not, and the reader's own choice wins over both.
 *
 * That choice is the caller's to hold. The credit is moved into MapLibre's own
 * cluster by a portal once the map reports having one, and that move remounts
 * this component — so a choice kept here would be reset by the map finishing
 * loading, silently undoing a press the reader had already made.
 */

import { IconInfoCircle } from "@tabler/icons-react";
import { useQuery } from "@tanstack/react-query";
import { useNarrowViewport } from "../lib/mediaQuery";

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
   * have said nothing and the viewport decides.
   *
   * Three states rather than two, because "not yet asked" is not the same
   * answer as "asked for it folded": only the first should give way when the
   * room available changes.
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
  /*
   * No choice yet means the viewport decides, and a choice outranks it from
   * then on. Reading the width once into a stored answer instead would freeze
   * whichever width the map first loaded at, and a phone that turns landscape
   * would keep hiding a line it now has room for.
   */
  const narrow = useNarrowViewport();
  const expanded = choice ?? !narrow;

  const credits = [attribution.data, extra].filter(Boolean).join(" · ");
  if (credits === "") {
    return null;
  }

  return (
    <div className="map-credits">
      <button
        className="map-credits__toggle"
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
        <IconInfoCircle size={12} stroke={1.2} aria-hidden="true" />
      </button>
      {expanded ? (
        <p className="map-credits__text" id={CREDIT_TEXT_ID}>
          {credits}
        </p>
      ) : null}
    </div>
  );
}
