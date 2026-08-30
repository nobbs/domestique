import { describe, expect, it } from "vitest";
import { CATALOGUE, GERMANY } from "./catalogue.generated";
import {
  covers,
  formatBytes,
  GERMAN_STATES,
  peakStagingBytes,
  redundant,
  search,
  select,
  toggle,
  transferBytes,
  unknown,
} from "./model";

/**
 * A copy of `runtimeconfig.regionSlug`. The catalogue is generated, so this is
 * the check that a regenerated one cannot start offering a region the service
 * would refuse to store.
 */
const REGION_SLUG = /^[a-z0-9]+(?:-[a-z0-9]+)*(?:\/[a-z0-9]+(?:-[a-z0-9]+)*)*$/;

describe("the catalogue", () => {
  it("offers only slugs the service accepts", () => {
    const refused = CATALOGUE.filter((entry) => !REGION_SLUG.test(entry.slug));

    expect(refused).toEqual([]);
  });

  it("holds Germany and its sixteen states", () => {
    expect(CATALOGUE.some((entry) => entry.slug === GERMANY)).toBe(true);
    expect(GERMAN_STATES).toHaveLength(16);
  });
});

describe("covers", () => {
  it("reads a country as holding its states", () => {
    expect(covers("europe/germany", "europe/germany/bayern")).toBe(true);
  });

  it("does not read a region as holding itself", () => {
    expect(covers("europe/germany", "europe/germany")).toBe(false);
  });

  it("does not mistake a shared prefix for containment", () => {
    expect(covers("europe/germa", "europe/germany")).toBe(false);
  });
});

describe("redundant", () => {
  it("names the state a selected country already contains", () => {
    expect(redundant(["europe/germany", "europe/germany/bayern"])).toEqual([
      "europe/germany/bayern",
    ]);
  });

  it("finds nothing wrong with sibling states", () => {
    expect(redundant(["europe/germany/bayern", "europe/germany/hessen"])).toEqual([]);
  });
});

describe("unknown", () => {
  it("keeps a slug the catalogue has never heard of", () => {
    expect(unknown(["europe/germany", "europe/germay"])).toEqual(["europe/germay"]);
  });
});

describe("the cost of a selection", () => {
  it("adds up what a rebuild downloads", () => {
    const states = ["europe/germany/bremen", "europe/germany/hamburg"];

    expect(transferBytes(states)).toBe(
      GERMAN_STATES.filter((state) => states.includes(state.slug)).reduce(
        (total, state) => total + (state.bytes ?? 0),
        0,
      ),
    );
  });

  it("stages only the largest region at once", () => {
    const germany = CATALOGUE.find((entry) => entry.slug === GERMANY);
    const selection = [GERMANY, "europe/germany/bremen"];

    expect(peakStagingBytes(selection)).toBe(germany?.bytes);
    expect(transferBytes(selection)).toBeGreaterThan(peakStagingBytes(selection));
  });

  it("counts an unknown region as costing nothing rather than failing", () => {
    expect(transferBytes(["europe/germay"])).toBe(0);
    expect(peakStagingBytes([])).toBe(0);
  });
});

describe("formatBytes", () => {
  it("reads a country in gigabytes and a city state in megabytes", () => {
    expect(formatBytes(4.5 * 1024 ** 3)).toBe("4.5 GB");
    expect(formatBytes(52 * 1024 ** 2)).toBe("52 MB");
    expect(formatBytes(null)).toBe("size unknown");
  });
});

describe("select", () => {
  it("drops the states a newly chosen country contains", () => {
    const chosen = select(["europe/germany/bayern", "europe/france"], "europe/germany");

    expect(chosen).toEqual(["europe/france", "europe/germany"]);
  });

  it("drops the country a newly chosen state sits in", () => {
    expect(select(["europe/germany"], "europe/germany/bayern")).toEqual(["europe/germany/bayern"]);
  });

  it("leaves a selection that already holds the region alone", () => {
    expect(select(["europe/germany"], "europe/germany")).toEqual(["europe/germany"]);
  });
});

describe("toggle", () => {
  it("removes a region that is already selected", () => {
    expect(toggle(["europe/germany"], "europe/germany")).toEqual([]);
  });
});

describe("search", () => {
  it("leads with Germany when nothing has been typed", () => {
    expect(search("").every((entry) => entry.slug.startsWith("europe/germany"))).toBe(true);
  });

  it("offers the states but not the districts inside them", () => {
    const offered = search("").map((entry) => entry.slug);

    expect(offered).toContain("europe/germany/bayern");
    expect(offered).not.toContain("europe/germany/bayern/oberbayern");
    expect(offered).toHaveLength(GERMAN_STATES.length + 1);
  });

  it("matches on the name as well as the slug", () => {
    // The slug spells it "wuerttemberg"; only the name carries the umlaut.
    expect(search("württemberg").map((entry) => entry.slug)).toContain(
      "europe/germany/baden-wuerttemberg",
    );
  });
});
