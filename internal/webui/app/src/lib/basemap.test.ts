import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { WebUIConfig } from "../api/types";
import { basemapStyleUrl, usePrefersDarkScheme } from "./basemap";

const LIGHT = "https://tiles.example.test/styles/liberty";
const DARK = "https://tiles.example.test/styles/liberty-dark";

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

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("basemapStyleUrl", () => {
  const config: WebUIConfig = { tileStyleUrl: LIGHT, tileStyleUrlDark: DARK };

  it("uses the light style under a light scheme", () => {
    expect(basemapStyleUrl(config, false)).toBe(LIGHT);
  });

  it("uses the dark style under a dark scheme", () => {
    expect(basemapStyleUrl(config, true)).toBe(DARK);
  });

  it("keeps the one style when no dark one is configured", () => {
    expect(basemapStyleUrl({ tileStyleUrl: LIGHT }, true)).toBe(LIGHT);
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
