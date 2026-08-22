import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Basemap, WebUIConfig } from "../api/types";
import { basemapFor, usePrefersDarkScheme } from "./basemap";

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
    expect(basemapFor(config, false)).toEqual({ styleUrl: LIGHT, dark: false });
  });

  it("uses the dark style under a dark scheme", () => {
    expect(basemapFor(config, true)).toEqual({ styleUrl: DARK, dark: true });
  });

  it("keeps the one style when no dark one is configured", () => {
    expect(basemapFor(configOf(oneStyle), true)).toEqual({ styleUrl: LIGHT, dark: false });
  });

  // The whole point of reporting darkness alongside the style rather than
  // letting a caller re-read the system scheme: here the two disagree, and
  // anything drawn over the map has to follow the style.
  it("does not report darkness for a light style under a dark scheme", () => {
    expect(basemapFor(configOf(oneStyle), true).dark).toBe(false);
  });

  // The other way the two disagree: imagery is dark ground at noon.
  it("reports darkness for dark cartography under a light scheme", () => {
    expect(basemapFor(configOf(satellite), false)).toEqual({ styleUrl: IMAGERY, dark: true });
  });

  it("loads the first entry, whatever else is offered", () => {
    expect(basemapFor(configOf(satellite, streets), true).styleUrl).toBe(IMAGERY);
  });

  // Unreachable through the service, which refuses an empty list at startup, and
  // unreachable through the parser, which refuses one on the wire. Asserted all
  // the same, because "there is always an entry" is a claim about two other
  // files rather than about this one, and a total function is what keeps a map
  // that lost its list from becoming a page that throws.
  it("names nothing where nothing is offered", () => {
    expect(basemapFor(configOf(), false)).toEqual({ styleUrl: "", dark: false });
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
