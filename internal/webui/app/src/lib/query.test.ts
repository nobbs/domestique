import { describe, expect, it } from "vitest";
import type { SuggestionSource } from "./query";
import { parseQuery, suggest, tokenValue, withoutToken, withToken } from "./query";

describe("parseQuery", () => {
  it("keeps plain words for the name match and applies nothing else", () => {
    const parsed = parseQuery("  rhine   valley ");

    expect(parsed.words).toBe("rhine valley");
    expect(parsed.tokens).toEqual([]);
    expect(parsed.sort).toBe("title");
    expect(parsed.direction).toBe("asc");
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
    expect(parseQuery("by distance")).toMatchObject({ sort: "distance", direction: "desc" });
    expect(parseQuery("rhine by up asc")).toMatchObject({
      sort: "ascent",
      direction: "asc",
      words: "rhine",
    });
    expect(parseQuery("BY near")).toMatchObject({ sort: "start", direction: "asc" });
    expect(parseQuery("by distance desc").tokens).toEqual([
      { key: "sort", text: "by distance desc" },
    ]);
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

describe("withToken", () => {
  it("replaces a key's token, keeping the words and the other tokens", () => {
    expect(withToken("rhine dist:10-20 up:<500", "dist", "40-80")).toBe("rhine up:<500 dist:40-80");
  });

  it("appends a token where there was none, and removes it for null", () => {
    expect(withToken("rhine", "sort", "by distance")).toBe("rhine by distance");
    expect(withToken("rhine by distance asc", "sort", null)).toBe("rhine");
  });

  it("writes every source of a list", () => {
    expect(withToken("src:komoot", "src", ["veloplanner", "local"])).toBe(
      "src:veloplanner src:local",
    );
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

describe("tokenValue", () => {
  it("writes ranges the parser reads back to the same bounds", () => {
    const range = { min: 42_300, max: null };
    const value = tokenValue.dist(range);

    expect(value).toBe(">42.3");
    expect(parseQuery(`dist:${value}`).filters.distanceMetres).toEqual(range);
    expect(tokenValue.time({ min: 5_400, max: 7_200 })).toBe("1h30-2h");
    expect(tokenValue.up({ min: null, max: null })).toBeNull();
  });

  it("leaves the default order unwritten, and a natural direction unsaid", () => {
    expect(tokenValue.sort("title", "asc")).toBeNull();
    expect(tokenValue.sort("title", "desc")).toBe("by name desc");
    expect(tokenValue.sort("distance", "desc")).toBe("by distance");
    expect(tokenValue.sort("distance", "asc")).toBe("by distance asc");
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
