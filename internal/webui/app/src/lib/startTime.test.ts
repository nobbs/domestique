import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  FORECAST_HORIZON_MS,
  FORECAST_PAST_ALLOWANCE_MS,
  isWithinForecastWindow,
  startTimeRefusal,
  useStartTime,
} from "./startTime";

/**
 * A `localStorage` for jsdom, which has none — see `basemap.test.ts` for why a
 * `Map` behind these three methods is enough. `removeItem` is the one method
 * the other preference hooks' stubs do not need: none of them clear their key.
 */
function stubStorage(): Map<string, string> {
  const entries = new Map<string, string>();
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => entries.get(key) ?? null,
    setItem: (key: string, value: string) => {
      entries.set(key, value);
    },
    removeItem: (key: string) => {
      entries.delete(key);
    },
  });

  return entries;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("isWithinForecastWindow", () => {
  const now = new Date("2026-08-24T12:00:00Z");

  it("accepts a time inside the window", () => {
    expect(isWithinForecastWindow(new Date("2026-08-25T09:00:00Z"), now)).toBe(true);
  });

  it("refuses a time more than 24 hours in the past", () => {
    const justOutside = new Date(now.getTime() - FORECAST_PAST_ALLOWANCE_MS - 1000);

    expect(isWithinForecastWindow(justOutside, now)).toBe(false);
  });

  it("refuses a time more than 16 days ahead", () => {
    const justOutside = new Date(now.getTime() + FORECAST_HORIZON_MS + 1000);

    expect(isWithinForecastWindow(justOutside, now)).toBe(false);
  });

  it("accepts the boundaries themselves", () => {
    expect(isWithinForecastWindow(new Date(now.getTime() - FORECAST_PAST_ALLOWANCE_MS), now)).toBe(
      true,
    );
    expect(isWithinForecastWindow(new Date(now.getTime() + FORECAST_HORIZON_MS), now)).toBe(true);
  });
});

describe("startTimeRefusal", () => {
  const now = new Date("2026-08-24T12:00:00Z");
  const hours = (count: number) => count * 60 * 60 * 1000;

  it("accepts a start whose whole ride fits inside the window", () => {
    expect(startTimeRefusal(new Date(now.getTime() + hours(2)), 4 * 3600, now)).toBeNull();
  });

  it("names a stale start as past rather than as outrunning the forecast", () => {
    expect(startTimeRefusal(new Date(now.getTime() - hours(30)), 3600, now)).toBe("past");
  });

  /*
   * The two refusals want opposite remedies — a stale start needs a later
   * time, one that outruns the forecast needs an earlier — so a ride that
   * departs inside the window and finishes outside it must not be told it is
   * in the past, however long it is.
   */
  it("names a long ride that finishes past the horizon as the horizon", () => {
    const departure = new Date(now.getTime() - hours(2));

    expect(startTimeRefusal(departure, 20 * 24 * 3600, now)).toBe("horizon");
  });

  it("names a departure past the horizon as the horizon too", () => {
    expect(startTimeRefusal(new Date(now.getTime() + hours(24 * 17)), 3600, now)).toBe("horizon");
  });
});

describe("useStartTime", () => {
  it("is unset before the reader picks anything", () => {
    stubStorage();
    const { result } = renderHook(() => useStartTime());

    expect(result.current[0]).toBeNull();
  });

  it("reports the pick, and remembers it for the next visit", () => {
    stubStorage();
    const { result } = renderHook(() => useStartTime());
    const chosen = new Date("2026-08-25T09:00:00Z");

    act(() => {
      result.current[1](chosen);
    });

    expect(result.current[0]).toEqual(chosen);
    // A second mounting is the reader coming back: it reads what the first
    // one wrote rather than starting over.
    expect(renderHook(() => useStartTime()).result.current[0]).toEqual(chosen);
  });

  it("clears the stored key when set back to null", () => {
    const entries = stubStorage();
    const { result } = renderHook(() => useStartTime());

    act(() => {
      result.current[1](new Date("2026-08-25T09:00:00Z"));
    });
    expect(entries.size).toBe(1);

    act(() => {
      result.current[1](null);
    });

    expect(entries.size).toBe(0);
    expect(result.current[0]).toBeNull();
  });

  // A start time from a previous visit must never come back once it has aged
  // out of the window the endpoint will accept — it would only ever be sent
  // as a request the endpoint is certain to refuse with a 400.
  it("reads a stale stored value as unset", () => {
    const entries = stubStorage();
    entries.set("domestique.start-time", "2020-01-01T00:00:00Z");

    const { result } = renderHook(() => useStartTime());

    expect(result.current[0]).toBeNull();
  });

  it("reads a far-future stored value as unset", () => {
    const entries = stubStorage();
    entries.set("domestique.start-time", "2099-01-01T00:00:00Z");

    const { result } = renderHook(() => useStartTime());

    expect(result.current[0]).toBeNull();
  });

  it("reads an unparseable stored value as unset", () => {
    const entries = stubStorage();
    entries.set("domestique.start-time", "not-a-time");

    const { result } = renderHook(() => useStartTime());

    expect(result.current[0]).toBeNull();
  });

  it("reads a valid stored value", () => {
    const entries = stubStorage();
    const soon = new Date(Date.now() + 60 * 60 * 1000).toISOString();
    entries.set("domestique.start-time", soon);

    const { result } = renderHook(() => useStartTime());

    expect(result.current[0]).toEqual(new Date(soon));
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
      removeItem: () => {
        throw new Error("denied");
      },
    });
    const { result } = renderHook(() => useStartTime());
    expect(result.current[0]).toBeNull();

    const chosen = new Date("2026-08-25T09:00:00Z");
    act(() => {
      result.current[1](chosen);
    });
    expect(result.current[0]).toEqual(chosen);
  });
});
