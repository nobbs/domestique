import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Basemap, WebUIConfig } from "../api/types";
import { basemapFor, useBasemapChoice, usePrefersDarkScheme } from "./basemap";

const LIGHT = "https://tiles.example.test/styles/liberty";
const DARK = "https://tiles.example.test/styles/liberty-dark";
const IMAGERY = "https://imagery.example.test/styles/hybrid";

/**
 * A `matchMedia` that answers one query and can change its mind.
 *
 * jsdom does not implement `matchMedia` at all, so the hook has nothing to read
 * without this. The listeners are held per stub rather than dispatched through a
 * real event target, which keeps `change` synchronous and the test free of
 * timers.
 */
function stubMatchMedia(matches: boolean) {
  const listeners = new Set<() => void>();
  const query = {
    matches,
    addEventListener: (_: string, listener: () => void) => {
      listeners.add(listener);
    },
    removeEventListener: (_: string, listener: () => void) => {
      listeners.delete(listener);
    },
  };
  vi.stubGlobal(
    "matchMedia",
    vi.fn(() => query),
  );

  return {
    listenerCount: () => listeners.size,
    change(next: boolean) {
      query.matches = next;
      for (const listener of listeners) {
        listener();
      }
    },
  };
}

/**
 * A `localStorage` for jsdom, which has none.
 *
 * The environment provides no storage at all, so a component that reads it
 * throws here rather than getting an empty one. A `Map` behind the two methods
 * the hook uses is enough, and keeping it in the test rather than in the shared
 * setup keeps every other suite reading a platform with no storage — which is
 * also a browser this code has to survive.
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

function configOf(...basemaps: Basemap[]): WebUIConfig {
  return { basemaps };
}

const streets: Basemap = {
  name: "Streets",
  styleUrl: LIGHT,
  styleUrlDark: DARK,
  darkCartography: false,
};
const oneStyle: Basemap = { name: "Streets", styleUrl: LIGHT, darkCartography: false };
const satellite: Basemap = { name: "Satellite", styleUrl: IMAGERY, darkCartography: true };

describe("basemapFor", () => {
  const config = configOf(streets);

  it("uses the light style under a light scheme", () => {
    expect(basemapFor(config, false)).toEqual({ name: "Streets", styleUrl: LIGHT, dark: false });
  });

  it("uses the dark style under a dark scheme", () => {
    expect(basemapFor(config, true)).toEqual({ name: "Streets", styleUrl: DARK, dark: true });
  });

  it("keeps the one style when no dark one is configured", () => {
    expect(basemapFor(configOf(oneStyle), true)).toEqual({
      name: "Streets",
      styleUrl: LIGHT,
      dark: false,
    });
  });

  // The whole point of reporting darkness alongside the style rather than
  // letting a caller re-read the system scheme: here the two disagree, and
  // anything drawn over the map has to follow the style.
  it("does not report darkness for a light style under a dark scheme", () => {
    expect(basemapFor(configOf(oneStyle), true).dark).toBe(false);
  });

  // The other way the two disagree: imagery is dark ground at noon.
  it("reports darkness for dark cartography under a light scheme", () => {
    expect(basemapFor(configOf(satellite), false)).toEqual({
      name: "Satellite",
      styleUrl: IMAGERY,
      dark: true,
    });
  });

  it("loads the first entry where the reader has picked nothing", () => {
    expect(basemapFor(configOf(satellite, streets), true).styleUrl).toBe(IMAGERY);
  });

  it("loads the entry the reader picked", () => {
    expect(basemapFor(configOf(streets, satellite), false, "Satellite")).toEqual({
      name: "Satellite",
      styleUrl: IMAGERY,
      dark: true,
    });
  });

  it("follows the scheme within the picked entry", () => {
    expect(basemapFor(configOf(satellite, streets), true, "Streets").styleUrl).toBe(DARK);
  });

  /*
   * A remembered name outlives the config that offered it: the operator may
   * have renamed or dropped the entry since. Falling back to the first is what
   * keeps that edit from leaving a returning reader with an empty map.
   */
  it("falls back to the first entry for a name no longer offered", () => {
    expect(basemapFor(configOf(streets, satellite), false, "Ordnance Survey").name).toBe("Streets");
  });

  // Unreachable through the service, which refuses an empty list at startup, and
  // unreachable through the parser, which refuses one on the wire. Asserted all
  // the same, because "there is always an entry" is a claim about two other
  // files rather than about this one, and a total function is what keeps a map
  // that lost its list from becoming a page that throws.
  it("names nothing where nothing is offered", () => {
    expect(basemapFor(configOf(), false)).toEqual({ name: "", styleUrl: "", dark: false });
  });
});

describe("useBasemapChoice", () => {
  it("has no choice before the reader makes one", () => {
    stubStorage();
    const { result } = renderHook(() => useBasemapChoice());

    expect(result.current[0]).toBeNull();
  });

  it("reports the pick, and remembers it for the next visit", () => {
    stubStorage();
    const { result } = renderHook(() => useBasemapChoice());

    act(() => {
      result.current[1]("Satellite");
    });

    expect(result.current[0]).toBe("Satellite");
    // A second mounting is the reader coming back: it reads what the first one
    // wrote rather than starting over.
    expect(renderHook(() => useBasemapChoice()).result.current[0]).toBe("Satellite");
  });

  // Every value stored, so the assertion fails if this ever starts keeping more
  // than the name of a basemap on a reader's machine.
  it("stores the name and nothing else", () => {
    const entries = stubStorage();
    const { result } = renderHook(() => useBasemapChoice());

    act(() => {
      result.current[1]("Satellite");
    });

    expect([...entries.values()]).toEqual(["Satellite"]);
  });

  /*
   * A private window or blocked storage throws on access rather than answering
   * nothing — and jsdom, where the rest of this suite runs, provides no storage
   * object at all. The pick still has to stand for as long as the page is open;
   * only its outliving the page is lost.
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
    const { result } = renderHook(() => useBasemapChoice());
    expect(result.current[0]).toBeNull();

    act(() => {
      result.current[1]("Satellite");
    });
    expect(result.current[0]).toBe("Satellite");
  });
});

describe("usePrefersDarkScheme", () => {
  it("reports the scheme in force on the first render", () => {
    stubMatchMedia(true);
    const { result } = renderHook(() => usePrefersDarkScheme());
    // On the first render, not after an effect: a map that built on the light
    // style and swapped it would flash and fetch a style document twice.
    expect(result.current).toBe(true);
  });

  it("follows a change of scheme", () => {
    const media = stubMatchMedia(false);
    const { result } = renderHook(() => usePrefersDarkScheme());
    expect(result.current).toBe(false);

    act(() => {
      media.change(true);
    });
    expect(result.current).toBe(true);
  });

  it("stops listening once unmounted", () => {
    const media = stubMatchMedia(false);
    const { unmount } = renderHook(() => usePrefersDarkScheme());
    expect(media.listenerCount()).toBe(1);

    unmount();
    expect(media.listenerCount()).toBe(0);
  });
});
