import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  arrivalWindow,
  DEFAULT_ALLOWANCE_SECONDS_PER_HOUR,
  formatAllowance,
  useStoppingAllowance,
} from "./stoppingAllowance";

/** A `localStorage` for jsdom, which has none — see `basemap.test.ts` for why. */
function stubStorage(entries = new Map<string, string>()): Map<string, string> {
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

describe("arrivalWindow", () => {
  it("adds the corpus quartiles to an hour of moving at the default allowance", () => {
    const window = arrivalWindow(3600, DEFAULT_ALLOWANCE_SECONDS_PER_HOUR);

    expect(window?.earliestSeconds).toBeCloseTo(3714, 6);
    expect(window?.latestSeconds).toBeCloseTo(4093, 6);
  });

  it("scales the whole window with the chosen allowance", () => {
    const window = arrivalWindow(3600, 2 * DEFAULT_ALLOWANCE_SECONDS_PER_HOUR);

    expect(window?.earliestSeconds).toBeCloseTo(3828, 6);
    expect(window?.latestSeconds).toBeCloseTo(4586, 6);
  });

  it("collapses onto the moving time when nothing is allowed for stopping", () => {
    expect(arrivalWindow(3600, 0)).toEqual({ earliestSeconds: 3600, latestSeconds: 3600 });
  });

  it("has no window without a predicted moving time", () => {
    expect(arrivalWindow(undefined, DEFAULT_ALLOWANCE_SECONDS_PER_HOUR)).toBeNull();
    expect(arrivalWindow(0, DEFAULT_ALLOWANCE_SECONDS_PER_HOUR)).toBeNull();
  });

  it("draws the rider's own quartiles where their rides measured them", () => {
    const window = arrivalWindow(3600, 300, {
      medianSecondsPerHour: 300,
      lowerQuartileSecondsPerHour: 180,
      upperQuartileSecondsPerHour: 900,
    });

    expect(window?.earliestSeconds).toBeCloseTo(3780, 6);
    expect(window?.latestSeconds).toBeCloseTo(4500, 6);
  });

  it("keeps the measured shape when the rider moves the allowance off their median", () => {
    const spread = {
      medianSecondsPerHour: 300,
      lowerQuartileSecondsPerHour: 180,
      upperQuartileSecondsPerHour: 900,
    };
    const window = arrivalWindow(3600, 600, spread);

    expect(window?.earliestSeconds).toBeCloseTo(3960, 6);
    expect(window?.latestSeconds).toBeCloseTo(5400, 6);
  });

  it("collapses onto the moving time for a rider whose rides never stop", () => {
    const window = arrivalWindow(3600, 266, {
      medianSecondsPerHour: 0,
      lowerQuartileSecondsPerHour: 0,
      upperQuartileSecondsPerHour: 0,
    });

    expect(window).toEqual({ earliestSeconds: 3600, latestSeconds: 3600 });
  });
});

describe("formatAllowance", () => {
  it("reads the allowance out in minutes", () => {
    expect(formatAllowance(266)).toBe("4.4 min");
  });
});

describe("useStoppingAllowance", () => {
  it("defaults to the corpus median before the rider chooses anything", () => {
    stubStorage();

    const { result } = renderHook(() => useStoppingAllowance());

    expect(result.current[0]).toBe(DEFAULT_ALLOWANCE_SECONDS_PER_HOUR);
  });

  it("remembers a choice across visits", () => {
    const entries = stubStorage();

    const { result } = renderHook(() => useStoppingAllowance());
    act(() => result.current[1](420));
    expect(result.current[0]).toBe(420);

    stubStorage(entries);
    expect(renderHook(() => useStoppingAllowance()).result.current[0]).toBe(420);
  });

  it("falls back to the default when a choice is not a number", () => {
    stubStorage();

    const { result } = renderHook(() => useStoppingAllowance());
    act(() => result.current[1](Number.NaN));

    expect(result.current[0]).toBe(266);
  });

  it("clamps a choice to the slider's domain", () => {
    stubStorage();

    const { result } = renderHook(() => useStoppingAllowance());
    act(() => result.current[1](9000));

    expect(result.current[0]).toBe(900);
  });

  it("falls back to the default where storage refuses to be read", () => {
    vi.stubGlobal("localStorage", {
      getItem: () => {
        throw new Error("blocked");
      },
      setItem: () => {
        throw new Error("blocked");
      },
    });

    const { result } = renderHook(() => useStoppingAllowance());
    expect(result.current[0]).toBe(DEFAULT_ALLOWANCE_SECONDS_PER_HOUR);

    // A refused write costs the choice its memory, not the page its window.
    act(() => result.current[1](420));
    expect(result.current[0]).toBe(420);
  });
});
