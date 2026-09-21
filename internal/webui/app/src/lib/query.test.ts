import { describe, expect, it } from "vitest";
import type { SuggestionSource } from "./query";
import { parseQuery, suggest, withoutToken } from "./query";

describe("parseQuery", () => {
  it("keeps plain words for the name match and applies nothing else", () => {
    const parsed = parseQuery("  rhine   valley ");

    expect(parsed.words).toBe("rhine valley");
    expect(parsed.tokens).toEqual([]);
    expect(parsed.order).toEqual([]);
    expect(parsed.drafts).toBeNull();
  });

  it("reads each range in its own unit", () => {
    const { filters, words } = parseQuery("loop dist:40-80.5 up:<1000 time:>1h30");

    expect(words).toBe("loop");
    expect(filters.distanceMetres).toEqual({ min: 40_000, max: 80_500 });
    expect(filters.ascentMetres).toEqual({ min: null, max: 1000 });
    expect(filters.movingSeconds).toEqual({ min: 5_400, max: null });
  });

  it("reads hours, minutes and bare minutes, a bare value being an upper bound", () => {
    expect(parseQuery("time:2h").filters.movingSeconds).toEqual({ min: null, max: 2 * 3600 });
    expect(parseQuery("time:<90m").filters.movingSeconds).toEqual({ min: null, max: 5_400 });
    expect(parseQuery("time:<=45").filters.movingSeconds).toEqual({ min: null, max: 2_700 });
  });

  it("collects every source, with the planner's alias", () => {
    expect(parseQuery("src:komoot SRC:planner").filters.providers).toEqual(["komoot", "local"]);
  });

  it("reads the order, each measure in its natural direction unless asc or desc follows", () => {
    expect(parseQuery("by distance").order).toEqual([{ column: "distance", direction: "desc" }]);
    expect(parseQuery("rhine by up asc")).toMatchObject({
      order: [{ column: "ascent", direction: "asc" }],
      words: "rhine",
    });
    expect(parseQuery("BY near").order).toEqual([{ column: "start", direction: "asc" }]);
    expect(parseQuery("by distance desc").tokens).toEqual([
      { key: "sort", text: "by distance desc" },
    ]);
  });

  it("reads each further by as a tiebreak, most significant first", () => {
    const parsed = parseQuery("by ascent desc loop by distance");

    expect(parsed.order).toEqual([
      { column: "ascent", direction: "desc" },
      { column: "distance", direction: "desc" },
    ]);
    expect(parsed.words).toBe("loop");
    expect(parsed.tokens.map((token) => token.text)).toEqual(["by ascent desc", "by distance"]);
  });

  it("keeps by as a word unless a measure follows, but not while it is still being typed", () => {
    expect(parseQuery("stand by me").words).toBe("stand by me");
    expect(parseQuery("rhine by").words).toBe("rhine");
  });

  it("reads draft and -draft", () => {
    expect(parseQuery("draft").drafts).toBe("only");
    expect(parseQuery("-draft").drafts).toBe("none");
  });

  it("ignores a known key whose value does not parse, so a half-typed token empties nothing", () => {
    const parsed = parseQuery("loop dist:far up:-3 time:");

    expect(parsed.words).toBe("loop");
    expect(parsed.tokens).toEqual([]);
    expect(parsed.filters.distanceMetres).toEqual({ min: null, max: null });
  });

  it("keeps an unknown key as a word, since it may be part of a name", () => {
    expect(parseQuery("col:du-galibier").words).toBe("col:du-galibier");
  });
});

describe("withoutToken", () => {
  it("removes a token spanning several words", () => {
    const text = "loop by distance asc src:komoot";
    const order = parseQuery(text).tokens[0];

    expect(order?.text).toBe("by distance asc");
    if (order) {
      expect(withoutToken(text, order)).toBe("loop src:komoot");
    }
  });

  it("removes exactly the token a chip names", () => {
    const text = "loop src:komoot src:local";
    const [komoot] = parseQuery(text).tokens;

    expect(komoot).toBeDefined();
    if (komoot) {
      expect(withoutToken(text, komoot)).toBe("loop src:local");
    }
  });
});

describe("suggest", () => {
  const source: SuggestionSource = {
    providers: [
      { provider: "veloplanner", label: "VeloPlanner", count: 6 },
      { provider: "local", label: "Planner", count: 1 },
    ],
    distances: [10_000, 20_000, 30_000, 40_000, 50_000, 60_000],
    ascents: [100, 300, 500, 700, 900, 1_100],
    durations: [1_800, 3_600, 5_400, 7_200, 9_000, 10_800],
  };
  const labels = (text: string) => suggest(text, source).map((entry) => entry.label);

  it("offers the keys a started word could be, and nothing for a finished word", () => {
    expect(labels("rhine d")).toEqual(["dist:", "draft"]);
    expect(labels("b")).toEqual(["by"]);
    expect(labels("")).toEqual([]);
    expect(labels("rhine ")).toEqual([]);
    expect(labels("draft")).toEqual([]);
  });

  it("leaves the cursor after a key, and a space after a whole token", () => {
    expect(suggest("rhine di", source)[0]?.query).toBe("rhine dist:");
    expect(suggest("rhine src:v", source)[0]?.query).toBe("rhine src:veloplanner ");
  });

  it("offers this library's sources, the planner by its alias", () => {
    expect(labels("src:")).toEqual(["src:veloplanner", "src:planner"]);
  });

  it("offers the measures after by, then the directions", () => {
    expect(labels("rhine by ")).toEqual([
      "by name",
      "by distance",
      "by ascent",
      "by time",
      "by steepest",
      "by near",
    ]);
    expect(suggest("by dis", source)[0]?.query).toBe("by distance ");
    expect(labels("by distance ")).toEqual(["asc", "desc"]);
    expect(labels("by distance d")).toEqual(["desc"]);
    expect(labels("stand by me ")).toEqual([]);
  });

  it("offers ranges cut from the library's own thirds, in friendly steps", () => {
    expect(labels("dist:")).toEqual(["dist:<30", "dist:30-50", "dist:>50"]);
    expect(labels("up:")).toEqual(["up:<500", "up:500-900", "up:>900"]);
    expect(labels("time:")).toEqual(["time:<1h30", "time:1h30-2h30", "time:>2h30"]);
  });

  it("offers no ranges for a library too small to cut in thirds", () => {
    expect(suggest("dist:", { ...source, distances: [10_000] })).toEqual([]);
  });
});
