import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { WebUIConfig } from "../api/types";
import { basemapFor, usePrefersDarkScheme } from "./basemap";

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

describe("basemapFor", () => {
  const config: WebUIConfig = { tileStyleUrl: LIGHT, tileStyleUrlDark: DARK };

  it("uses the light style under a light scheme", () => {
    expect(basemapFor(config, false)).toEqual({ styleUrl: LIGHT, dark: false });
  });

  it("uses the dark style under a dark scheme", () => {
    expect(basemapFor(config, true)).toEqual({ styleUrl: DARK, dark: true });
  });

  it("keeps the one style when no dark one is configured", () => {
    expect(basemapFor({ tileStyleUrl: LIGHT }, true)).toEqual({ styleUrl: LIGHT, dark: false });
  });

  // The whole point of reporting darkness alongside the style rather than
  // letting a caller re-read the system scheme: here the two disagree, and
  // anything drawn over the map has to follow the style.
  it("does not report darkness for a light style under a dark scheme", () => {
    expect(basemapFor({ tileStyleUrl: LIGHT }, true).dark).toBe(false);
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
