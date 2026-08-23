import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  distanceLabel,
  distanceUnitLabel,
  elevationUnitLabel,
  elevationValue,
  metresToFeet,
  metresToMiles,
  useUnitSystem,
} from "./units";

/** A `localStorage` for jsdom, which has none. See `basemap.test.ts` for why a `Map` behind the two methods the hook uses is enough. */
function stubStorage(): Map<string, string> {
  const entries = new Map<string, string>();
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => entries.get(key) ?? null,
    setItem: (key: string, value: string) => {
      entries.set(key, value);
    },
  });

  return entries;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("metresToFeet", () => {
  it("converts a metre distance to feet", () => {
    expect(metresToFeet(1000)).toBeCloseTo(3280.84, 1);
  });
});

describe("metresToMiles", () => {
  it("converts a metre distance to miles", () => {
    expect(metresToMiles(1609.344)).toBeCloseTo(1, 6);
  });
});

describe("distanceUnitLabel", () => {
  it("names the metric and imperial long-distance units", () => {
    expect(distanceUnitLabel("metric")).toBe("km");
    expect(distanceUnitLabel("imperial")).toBe("mi");
  });
});

describe("elevationUnitLabel", () => {
  it("names the metric and imperial height units", () => {
    expect(elevationUnitLabel("metric")).toBe("m");
    expect(elevationUnitLabel("imperial")).toBe("ft");
  });
});

describe("elevationValue", () => {
  it("passes a metric height through unchanged", () => {
    expect(elevationValue(500, "metric")).toBe(500);
  });

  it("converts an imperial height to feet", () => {
    expect(elevationValue(500, "imperial")).toBeCloseTo(1640.4, 1);
  });
});

describe("distanceLabel", () => {
  it("reads a whole-route step as whole kilometres", () => {
    expect(distanceLabel(42_000, 10, "metric")).toBe("42");
  });

  it("gains decimals as the step narrows, in metric", () => {
    expect(distanceLabel(1500, 0.5, "metric")).toBe("1.5");
  });

  it("converts both the value and the step for imperial, keeping the gridline", () => {
    // A one-kilometre step reads as a whole number of kilometres in metric,
    // but the same step is well under a mile — so relabelled in miles it
    // needs a decimal to still tell its neighbours apart.
    expect(distanceLabel(42_000, 1, "imperial")).toBe("26.1");
  });
});

describe("useUnitSystem", () => {
  it("is metric before the reader picks anything", () => {
    stubStorage();
    const { result } = renderHook(() => useUnitSystem());

    expect(result.current[0]).toBe("metric");
  });

  it("reports the pick, and remembers it for the next visit", () => {
    stubStorage();
    const { result } = renderHook(() => useUnitSystem());

    act(() => {
      result.current[1]("imperial");
    });

    expect(result.current[0]).toBe("imperial");
    expect(renderHook(() => useUnitSystem()).result.current[0]).toBe("imperial");
  });

  it("ignores a corrupted stored value rather than throwing", () => {
    const entries = stubStorage();
    entries.set("domestique.units", "furlongs");

    const { result } = renderHook(() => useUnitSystem());

    expect(result.current[0]).toBe("metric");
  });

  /*
   * A private window or blocked storage throws on access rather than
   * answering nothing — and jsdom, where the rest of this suite runs,
   * provides no storage object at all. The pick still has to stand for as
   * long as the page is open; only its outliving the page is lost.
   */
  it("keeps working where the browser refuses storage", () => {
    vi.stubGlobal("localStorage", {
      getItem: () => {
        throw new Error("denied");
      },
      setItem: () => {
        throw new Error("denied");
      },
    });
    const { result } = renderHook(() => useUnitSystem());
    expect(result.current[0]).toBe("metric");

    act(() => {
      result.current[1]("imperial");
    });
    expect(result.current[0]).toBe("imperial");
  });
});
