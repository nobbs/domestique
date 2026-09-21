/**
 * The search palette's query language: free words match names, and `key:value`
 * tokens narrow and order the library.
 *
 *   kaiserstuhl dist:40-80 up:<1000 time:<3h src:komoot by distance desc
 *
 * Distances are kilometres, ascent metres, times `2h`, `90m` or `1h30`. A range
 * is `a-b`, `<b`, `>a`, or a bare `b` for at most b. `by x` orders by a measure in
 * its natural direction, `by x asc` or `by x desc` either way, and each further
 * `by` breaks the ties of the ones before it; `by` followed by anything else is a
 * word. A known key whose value does not parse, half-typed or
 * mistyped, is ignored rather than matched against names, which would empty the
 * list while it is being written.
 */

import type { LibraryFilters, NumericRange } from "./filters";
import { EMPTY_FILTERS } from "./filters";
import type { SortColumn, SortDirection } from "./ranking";
import { initialDirection } from "./ranking";

/** Which part of the query a token sets. */
export type TokenKey = "dist" | "up" | "time" | "src" | "sort" | "draft";

export interface Token {
  key: TokenKey;
  /** The token exactly as typed, for chips and for removing it again. */
  text: string;
}

export interface ParsedQuery {
  /** What is left for matching names, tokens removed. */
  words: string;
  filters: LibraryFilters;
  /** The order, most significant first; empty is by name, the order matching leaves. */
  order: Array<{ column: SortColumn; direction: SortDirection }>;
  /** `draft` lists only drafts, `-draft` none; absent, drafts sit on top as usual. */
  drafts: "only" | "none" | null;
  tokens: Token[];
}

const SORT_NAMES: Record<string, SortColumn> = {
  name: "title",
  distance: "distance",
  dist: "distance",
  ascent: "ascent",
  up: "ascent",
  climbing: "ascent",
  time: "movingTime",
  steepest: "gradient",
  max: "gradient",
  near: "start",
  nearest: "start",
};

/** The spelling completions offer for each column. */
const SORT_SPELLING: Record<SortColumn, string> = {
  title: "name",
  distance: "distance",
  ascent: "ascent",
  movingTime: "time",
  gradient: "steepest",
  start: "near",
};

const SOURCE_ALIASES: Record<string, string> = { planner: "local" };

const KNOWN_KEYS = new Set(["dist", "up", "time", "src"]);

/** Seconds in `2h`, `90m`, `1h30` or `1h30m`; null for anything else. */
function parseDuration(value: string): number | null {
  const match = /^(?:(\d+(?:\.\d+)?)h)?(?:(\d+)m?)?$/.exec(value);
  if (!match || (match[1] === undefined && match[2] === undefined)) {
    return null;
  }
  const hours = Number(match[1] ?? 0);
  const minutes = Number(match[2] ?? 0);
  // A bare number is minutes; after hours, the digits are minutes too.
  return Math.round(hours * 3600 + minutes * 60);
}

function parseNumber(value: string): number | null {
  return /^\d+(?:\.\d+)?$/.test(value) ? Number(value) : null;
}

/** `a-b`, `<b`, `>a` (`<=`/`>=` read the same) or a bare `b`, each side through `read`. */
function parseRange(value: string, read: (side: string) => number | null): NumericRange | null {
  const bound = /^([<>])=?(.+)$/.exec(value);
  if (bound?.[1] && bound[2]) {
    const side = read(bound[2]);
    if (side === null) {
      return null;
    }
    return bound[1] === "<" ? { min: null, max: side } : { min: side, max: null };
  }
  const [low, high, ...rest] = value.split("-");
  if (low !== undefined && high === undefined) {
    const max = read(low);
    return max === null ? null : { min: null, max };
  }
  if (rest.length > 0 || low === undefined || high === undefined) {
    return null;
  }
  const min = read(low);
  const max = read(high);

  return min === null || max === null ? null : { min, max };
}

const kilometres = (side: string) => {
  const km = parseNumber(side);
  return km === null ? null : Math.round(km * 1000);
};

/** The filter each range key bounds, and how its values read. */
const RANGE_KEYS: Partial<
  Record<
    string,
    {
      field: "distanceMetres" | "ascentMetres" | "movingSeconds";
      read: (side: string) => number | null;
    }
  >
> = {
  dist: { field: "distanceMetres", read: kilometres },
  up: { field: "ascentMetres", read: parseNumber },
  time: { field: "movingSeconds", read: parseDuration },
};

/** The query as the palette applies it. */
export function parseQuery(text: string): ParsedQuery {
  const filters: LibraryFilters = { ...EMPTY_FILTERS, providers: [] };
  const order: ParsedQuery["order"] = [];
  let drafts: ParsedQuery["drafts"] = null;
  const tokens: Token[] = [];
  const words: string[] = [];

  const list = text.split(/\s+/).filter(Boolean);
  for (let index = 0; index < list.length; index++) {
    const word = list[index] as string;
    const lower = word.toLowerCase();
    const column = lower === "by" ? SORT_NAMES[(list[index + 1] ?? "").toLowerCase()] : undefined;
    if (column) {
      const next = (list[index + 2] ?? "").toLowerCase();
      const explicit = next === "asc" || next === "desc" ? next : null;
      const span = list.slice(index, index + (explicit ? 3 : 2));
      order.push({ column, direction: explicit ?? initialDirection(column) });
      tokens.push({ key: "sort", text: span.join(" ") });
      index += span.length - 1;
      continue;
    }
    // A trailing `by` is an order being written, not a word to match names with.
    if (lower === "by" && index === list.length - 1) {
      continue;
    }
    if (lower === "draft" || lower === "-draft") {
      drafts = lower === "draft" ? "only" : "none";
      tokens.push({ key: "draft", text: word });
      continue;
    }
    const colon = lower.indexOf(":");
    const key = colon > 0 ? lower.slice(0, colon) : "";
    const value = lower.slice(colon + 1);
    const ranged = RANGE_KEYS[key];
    const range = ranged ? parseRange(value, ranged.read) : null;
    if (ranged && range) {
      filters[ranged.field] = range;
    } else if (key === "src" && value !== "") {
      filters.providers = [...filters.providers, SOURCE_ALIASES[value] ?? value];
    } else {
      if (!KNOWN_KEYS.has(key)) {
        words.push(word);
      }
      continue;
    }
    tokens.push({ key: key as TokenKey, text: word });
  }

  return { words: words.join(" "), filters, order, drafts, tokens };
}

function formatKilometres(metres: number): string {
  return String(Math.round(metres / 100) / 10);
}

function formatDuration(seconds: number): string {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.round((seconds % 3600) / 60);
  if (hours === 0) {
    return `${minutes}m`;
  }
  return minutes === 0 ? `${hours}h` : `${hours}h${minutes}`;
}

/**
 * The query as the search field shows it: each finished token as a chip, and
 * what is left as text. A token still at the end of the query, with nothing typed
 * after it, is not finished — `by distance` may yet take `asc` — so it stays text.
 */
export function splitQuery(text: string): { chips: Token[]; draft: string } {
  const words = text.split(/\s+/).filter(Boolean);
  const trailing = /\s$/.test(text);
  const { tokens } = parseQuery(text);
  const chips: Token[] = [];
  const rest: string[] = [];
  let at = 0;
  let next = 0;
  while (at < words.length) {
    const token = tokens[next];
    const span = token?.text.split(" ") ?? [];
    if (token && span.every((part, offset) => words[at + offset] === part)) {
      const end = at + span.length;
      if (end < words.length || trailing) {
        chips.push(token);
      } else {
        rest.push(...span);
      }
      at = end;
      next += 1;
    } else {
      rest.push(words[at] as string);
      at += 1;
    }
  }

  return { chips, draft: `${rest.join(" ")}${trailing && rest.length > 0 ? " " : ""}` };
}

/** The whole query from the field's chips and its text, the inverse of `splitQuery`. */
export function joinQuery(chips: Token[], draft: string): string {
  return chips.length === 0 ? draft : `${chips.map((chip) => chip.text).join(" ")} ${draft}`;
}

/** The query without one token exactly as it was typed, say from its chip. */
export function withoutToken(text: string, token: Token): string {
  const words = text.split(/\s+/).filter(Boolean);
  const span = token.text.split(" ");
  const at = words.findIndex((_, start) =>
    span.every((part, offset) => words[start + offset] === part),
  );

  return (at < 0 ? words : [...words.slice(0, at), ...words.slice(at + span.length)]).join(" ");
}

/** One completion for the word being typed: what to show, and the query it leaves. */
export interface Suggestion {
  label: string;
  /** A word or two, to fit many in a row. */
  hint: string;
  /** The longer explanation, for a tooltip. */
  example?: string;
  query: string;
}

/** What the library holds, for completions that are about this library rather than any. */
export interface SuggestionSource {
  providers: ReadonlyArray<{ provider: string; label: string; count: number }>;
  distances: readonly number[];
  ascents: readonly number[];
  durations: readonly number[];
  /** Whether the reader has drafts, and so a use for the `draft` key. */
  drafts: boolean;
}

const KEYS: ReadonlyArray<{ key: string; hint: string; example: string }> = [
  { key: "dist:", hint: "km", example: "Distance in km, e.g. dist:40-80" },
  { key: "up:", hint: "climb m", example: "Ascent in m, e.g. up:<1000" },
  { key: "time:", hint: "moving", example: "Moving time, e.g. time:<2h" },
  { key: "src:", hint: "source", example: "Source, e.g. src:komoot" },
  { key: "by", hint: "order", example: "Order, e.g. by distance desc by name" },
  { key: "draft", hint: "drafts", example: "Only drafts; -draft hides them" },
];

/** The value a third of the way and two thirds of the way through, rounded to `step`. */
function thirds(values: readonly number[], step: number): [number, number] | null {
  const sorted = values.filter((value) => value > 0).sort((a, b) => a - b);
  if (sorted.length < 3) {
    return null;
  }
  const at = (share: number) =>
    Math.max(step, Math.round((sorted[Math.floor(sorted.length * share)] ?? 0) / step) * step);
  const low = at(1 / 3);
  const high = at(2 / 3);

  return high > low ? [low, high] : null;
}

function rangeSuggestions(
  key: string,
  bounds: [number, number] | null,
  write: (value: number) => string,
  noun: string,
): Array<{ value: string; hint: string }> {
  if (!bounds) {
    return [];
  }
  const [low, high] = bounds.map(write);
  return [
    { value: `<${low}`, hint: `shorter ${noun}` },
    { value: `${low}-${high}`, hint: `middle ${noun}` },
    { value: `>${high}`, hint: `longer ${noun}` },
  ].map((entry) => ({ ...entry, value: `${key}:${entry.value}` }));
}

/** The keys this reader has a use for. */
function keysFor(source: SuggestionSource) {
  return KEYS.filter(({ key }) => key !== "draft" || source.drafts);
}

/** Every key the query does not hold yet, to follow `text`, which ends where a word would begin. */
function freshKeys(text: string, source: SuggestionSource): Suggestion[] {
  const held = new Set(parseQuery(text).tokens.map((token) => token.key));
  return keysFor(source)
    .filter(({ key }) => !["dist:", "up:", "time:", "draft"].includes(key) || !held.has(keyOf(key)))
    .map(({ key, hint, example }) => ({
      label: key,
      hint,
      example,
      query: `${text}${key}${key.endsWith(":") ? "" : " "}`,
    }));
}

/** The token key a completion key names: `dist:` names `dist`, `by` names `sort`. */
function keyOf(key: string): TokenKey {
  return key === "by" ? "sort" : (key.replace(":", "") as TokenKey);
}

/**
 * Completions for the last word of the query, best first; with no word begun,
 * the keys the query does not already hold. Accepting one replaces that word.
 */
export function suggest(text: string, source: SuggestionSource): Suggestion[] {
  const direction = /(^|\s)by\s+(\S+)\s+(\S*)$/i.exec(text);
  if (direction?.[2] && SORT_NAMES[direction[2].toLowerCase()]) {
    const partial = (direction[3] ?? "").toLowerCase();
    const before = text.slice(0, text.length - partial.length);
    const directions = (["asc", "desc"] as const)
      .filter((each) => each.startsWith(partial) && each !== partial)
      .map((each) => ({
        label: each,
        hint: each === "asc" ? "ascending" : "descending",
        query: `${before}${each} `,
      }));
    // Nothing begun after the measure: a direction, or the next filter.
    return partial === "" ? [...directions, ...freshKeys(text, source)] : directions;
  }
  const measure = /(^|\s)by\s+(\S*)$/i.exec(text);
  if (measure) {
    const partial = (measure[2] ?? "").toLowerCase();
    const before = text.slice(0, text.length - partial.length);
    return Object.entries(SORT_SPELLING)
      .filter(([, spelling]) => spelling.startsWith(partial) && spelling !== partial)
      .map(([column, spelling]) => ({
        label: `by ${spelling}`,
        hint:
          column === "title" ? "A to Z" : column === "start" ? "nearest first" : "largest first",
        query: `${before}${spelling} `,
      }));
  }
  const offered = keysFor(source);
  const match = /(^|\s)(\S+)$/.exec(text);
  const word = match?.[2];
  if (word === undefined) {
    return freshKeys(text, source);
  }
  const before = text.slice(0, text.length - word.length);
  const lower = word.toLowerCase();
  const colon = lower.indexOf(":");
  let options: Array<{ value: string; hint: string; example?: string; final: boolean }>;

  if (colon < 0) {
    options = offered
      .filter(({ key }) => key.startsWith(lower) && key !== lower)
      .map(({ key, hint, example }) => ({ value: key, hint, example, final: !key.endsWith(":") }));
  } else {
    const key = lower.slice(0, colon);
    let values: Array<{ value: string; hint: string }> = [];
    if (key === "src") {
      values = source.providers.map(({ provider, label, count }) => ({
        value: `src:${provider === "local" ? "planner" : provider}`,
        hint: `${label}, ${count} ${count === 1 ? "route" : "routes"}`,
      }));
    } else if (key === "dist") {
      values = rangeSuggestions(key, thirds(source.distances, 5_000), formatKilometres, "rides");
    } else if (key === "up") {
      values = rangeSuggestions(key, thirds(source.ascents, 100), String, "climbing");
    } else if (key === "time") {
      values = rangeSuggestions(key, thirds(source.durations, 1_800), formatDuration, "rides");
    }
    options = values
      .filter(({ value }) => value.startsWith(lower) && value !== lower)
      .map((entry) => ({ ...entry, final: true }));
  }

  return options.map(({ value, hint, example, final }) => ({
    label: value,
    hint,
    ...(example === undefined ? {} : { example }),
    query: `${before}${value}${final ? " " : ""}`,
  }));
}
