import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type {
  nextThemeChoice as NextThemeChoice,
  resolvesDark as ResolvesDark,
  useThemeChoice as UseThemeChoice,
} from "./theme";

/**
 * A `localStorage` for jsdom, which has none — see `basemap.test.ts` for why
 * a `Map` behind the two methods the hook uses is enough.
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

// A pick outranks storage for as long as the module lives, so a test that
// makes one would decide what the next test reads. Each takes its own module
// instance instead — the same reset `identity.test.tsx` does, for the same
// kind of store.
async function freshTheme(): Promise<{
  nextThemeChoice: typeof NextThemeChoice;
  resolvesDark: typeof ResolvesDark;
  useThemeChoice: typeof UseThemeChoice;
}> {
  vi.resetModules();
  return import("./theme");
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("resolvesDark", () => {
  it('defers to the system for "system"', async () => {
    const { resolvesDark } = await freshTheme();

    expect(resolvesDark("system", true)).toBe(true);
    expect(resolvesDark("system", false)).toBe(false);
  });

  it("holds regardless of the system for an explicit pick", async () => {
    const { resolvesDark } = await freshTheme();

    expect(resolvesDark("dark", false)).toBe(true);
    expect(resolvesDark("light", true)).toBe(false);
  });
});

describe("nextThemeChoice", () => {
  it("steps through every choice and comes back round", async () => {
    const { nextThemeChoice } = await freshTheme();

    expect(nextThemeChoice("system")).toBe("light");
    expect(nextThemeChoice("light")).toBe("dark");
    expect(nextThemeChoice("dark")).toBe("system");
  });
});

describe("useThemeChoice", () => {
  it("defaults to following the system before the reader picks anything", async () => {
    stubStorage();
    const { useThemeChoice } = await freshTheme();
    const { result } = renderHook(() => useThemeChoice());

    expect(result.current[0]).toBe("system");
  });

  it("reports the pick, and remembers it for the next visit", async () => {
    stubStorage();
    const { useThemeChoice } = await freshTheme();
    const { result } = renderHook(() => useThemeChoice());

    act(() => {
      result.current[1]("dark");
    });

    expect(result.current[0]).toBe("dark");
    // A second mounting is the reader coming back: it reads what the first one
    // wrote rather than starting over.
    expect(renderHook(() => useThemeChoice()).result.current[0]).toBe("dark");
  });

  /*
   * The bar's toggle and the `data-theme` attribute are two consumers on one
   * screen, so a pick made through either has to reach the other where it
   * already stands, rather than on its next mount.
   */
  it("reaches a consumer already mounted elsewhere", async () => {
    stubStorage();
    const { useThemeChoice } = await freshTheme();
    const toggle = renderHook(() => useThemeChoice());
    const document = renderHook(() => useThemeChoice());

    act(() => {
      toggle.result.current[1]("light");
    });

    expect(document.result.current[0]).toBe("light");
  });

  // Every value stored, so the assertion fails if this ever starts keeping
  // more than the one word on a reader's machine.
  it("stores the choice and nothing else", async () => {
    const entries = stubStorage();
    const { useThemeChoice } = await freshTheme();
    const { result } = renderHook(() => useThemeChoice());

    act(() => {
      result.current[1]("light");
    });

    expect([...entries.values()]).toEqual(["light"]);
  });

  // A value from an older or unrelated build must not read as a pick this
  // build never offered.
  it("falls back to system for a stored value it does not recognise", async () => {
    const entries = stubStorage();
    entries.set("domestique.theme", "midnight");
    const { useThemeChoice } = await freshTheme();
    const { result } = renderHook(() => useThemeChoice());

    expect(result.current[0]).toBe("system");
  });

  /*
   * A private window or blocked storage throws on access rather than
   * answering nothing — and jsdom, where the rest of this suite runs,
   * provides no storage object at all. The pick still has to stand for as
   * long as the page is open; only its outliving the page is lost.
   */
  it("keeps working where the browser refuses storage", async () => {
    vi.stubGlobal("localStorage", {
      getItem: () => {
        throw new Error("denied");
      },
      setItem: () => {
        throw new Error("denied");
      },
    });
    const { useThemeChoice } = await freshTheme();
    const { result } = renderHook(() => useThemeChoice());
    expect(result.current[0]).toBe("system");

    act(() => {
      result.current[1]("dark");
    });
    expect(result.current[0]).toBe("dark");
  });
});
