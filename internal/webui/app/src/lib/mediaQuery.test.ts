import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useMediaQuery, usePrefersReducedMotion } from "./mediaQuery";

const QUERY = "(min-width: 40rem)";

/**
 * A `matchMedia` that answers one query and can change its mind.
 *
 * jsdom implements none, so the hook has nothing to read without this. The
 * listeners are held per stub rather than dispatched through a real event
 * target, which keeps `change` synchronous and the test free of timers.
 */
function stubMatchMedia(matches: boolean) {
  const listeners = new Set<() => void>();
  const list = {
    matches,
    addEventListener: (_: string, listener: () => void) => {
      listeners.add(listener);
    },
    removeEventListener: (_: string, listener: () => void) => {
      listeners.delete(listener);
    },
  };
  const matchMedia = vi.fn((_query: string) => list);
  vi.stubGlobal("matchMedia", matchMedia);

  return {
    asked: () => matchMedia.mock.calls.map(([query]) => query),
    listenerCount: () => listeners.size,
    change(next: boolean) {
      list.matches = next;
      for (const listener of listeners) {
        listener();
      }
    },
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("useMediaQuery", () => {
  it("reports the match on the first render rather than after it", () => {
    stubMatchMedia(true);

    // An effect that seeded state would render `false` once and correct itself,
    // which is a visible flash in everything that reads this.
    expect(renderHook(() => useMediaQuery(QUERY)).result.current).toBe(true);
  });

  it("re-renders when the match changes", () => {
    const media = stubMatchMedia(false);
    const { result } = renderHook(() => useMediaQuery(QUERY));

    act(() => media.change(true));

    expect(result.current).toBe(true);
  });

  it("stops listening once nothing is asking", () => {
    const media = stubMatchMedia(false);
    const { unmount } = renderHook(() => useMediaQuery(QUERY));
    expect(media.listenerCount()).toBe(1);

    unmount();

    expect(media.listenerCount()).toBe(0);
  });
});

describe("usePrefersReducedMotion", () => {
  it("asks for the reduced-motion preference", () => {
    const media = stubMatchMedia(true);

    const { result } = renderHook(() => usePrefersReducedMotion());

    expect(result.current).toBe(true);
    expect(media.asked()).toContain("(prefers-reduced-motion: reduce)");
  });
});
