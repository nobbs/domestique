import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Route } from "../api/types";
import { useSeenRoutes } from "./seenRoutes";

function stage(overrides: Partial<Route> = {}): Route {
  return {
    provider: "veloplanner",
    sourceRouteId: 12,
    stageOrder: 1,
    title: "Alpine loop",
    sourceRouteName: "Alpine loop",
    routeName: "Ascent",
    sourceRevision: "2026-08-17",
    contentHash: "hash",
    distanceMetres: 42_500,
    ascentMetres: 620,
    maxGradientPercent: 11.4,
    pointCount: 1200,
    ...overrides,
  };
}

/**
 * A `localStorage` for jsdom, which has none. See `basemap.test.ts` for why a
 * `Map` behind the two methods the hook uses is enough.
 */
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

describe("useSeenRoutes", () => {
  it("calls a stage new before it has ever been marked seen", () => {
    stubStorage();
    const { result } = renderHook(() => useSeenRoutes());

    expect(result.current.changeOf(stage())).toBe("new");
  });

  it("calls a marked stage unchanged once its revision has been seen", () => {
    stubStorage();
    const { result } = renderHook(() => useSeenRoutes());

    act(() => {
      result.current.markSeen(stage());
    });

    expect(result.current.changeOf(stage())).toBeNull();
  });

  // The revision the service already uses to decide a target needs rewriting,
  // so a stage the source has revised is the same signal this reads.
  it("calls a marked stage updated once its revision moves on", () => {
    stubStorage();
    const { result } = renderHook(() => useSeenRoutes());

    act(() => {
      result.current.markSeen(stage({ sourceRevision: "2026-08-17" }));
    });

    expect(result.current.changeOf(stage({ sourceRevision: "2026-08-24" }))).toBe("updated");
  });

  it("remembers a mark for the next visit", () => {
    stubStorage();
    const { result } = renderHook(() => useSeenRoutes());

    act(() => {
      result.current.markSeen(stage());
    });

    expect(renderHook(() => useSeenRoutes()).result.current.changeOf(stage())).toBeNull();
  });

  it("tells two stages apart by their full identity, not just a route id", () => {
    stubStorage();
    const { result } = renderHook(() => useSeenRoutes());

    act(() => {
      result.current.markSeen(stage({ stageOrder: 1 }));
    });

    expect(result.current.changeOf(stage({ stageOrder: 2 }))).toBe("new");
  });

  // Cleared storage — a reader who wipes site data, or a private window that
  // never wrote anything back — must read exactly as a first visit does.
  it("treats cleared storage as a first visit", () => {
    const entries = stubStorage();
    const { result: first } = renderHook(() => useSeenRoutes());
    act(() => {
      first.current.markSeen(stage());
    });
    entries.clear();

    const { result: second } = renderHook(() => useSeenRoutes());
    expect(second.current.changeOf(stage())).toBe("new");
  });

  it("ignores a corrupted stored value rather than throwing", () => {
    const entries = stubStorage();
    entries.set("domestique.seen-routes", "not json");

    const { result } = renderHook(() => useSeenRoutes());

    expect(result.current.changeOf(stage())).toBe("new");
  });

  it("ignores a stored value that parses but is not the shape this reads", () => {
    const entries = stubStorage();
    entries.set("domestique.seen-routes", JSON.stringify({ "veloplanner/12/1": 42 }));

    const { result } = renderHook(() => useSeenRoutes());

    expect(result.current.changeOf(stage())).toBe("new");
  });

  /*
   * A private window or blocked storage throws on access rather than
   * answering nothing — and jsdom, where the rest of this suite runs,
   * provides no storage object at all. The mark still has to stand for as
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
    const { result } = renderHook(() => useSeenRoutes());
    expect(result.current.changeOf(stage())).toBe("new");

    act(() => {
      result.current.markSeen(stage());
    });
    expect(result.current.changeOf(stage())).toBeNull();
  });
});
