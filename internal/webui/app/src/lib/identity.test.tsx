import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { webUIConfigQuery } from "../api/queries";
import type { WebUIConfig } from "../api/types";
import type {
  useEffectiveAdmin as UseEffectiveAdmin,
  useViewAsRider as UseViewAsRider,
} from "./identity";

/** A `localStorage` for jsdom, which has none. See `basemap.test.ts` for why a `Map` behind the two methods the hook uses is enough. */
function stubStorage(): void {
  const entries = new Map<string, string>();
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => entries.get(key) ?? null,
    setItem: (key: string, value: string) => {
      entries.set(key, value);
    },
  });
}

// identity.ts reads localStorage into a module-level variable exactly once,
// so each test needs its own fresh module instance to observe a mount-time
// read rather than a leftover value from an earlier test.
async function freshIdentity(): Promise<{
  useEffectiveAdmin: typeof UseEffectiveAdmin;
  useViewAsRider: typeof UseViewAsRider;
}> {
  vi.resetModules();
  return import("./identity");
}

afterEach(() => {
  vi.unstubAllGlobals();
});

function config(admin: boolean): WebUIConfig {
  return {
    basemaps: [],
    sourceBaseUrls: {},
    timezone: "Europe/Berlin",
    identity: { display: "rider@example.test", admin },
  };
}

function wrapWith(value?: WebUIConfig) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  if (value) {
    client.setQueryData(webUIConfigQuery().queryKey, value);
  }

  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
}

describe("useEffectiveAdmin", () => {
  it("is true for an admin with the rider view off", async () => {
    stubStorage();
    const { useEffectiveAdmin } = await freshIdentity();
    const { result } = renderHook(() => useEffectiveAdmin(), { wrapper: wrapWith(config(true)) });

    expect(result.current).toBe(true);
  });

  it("is false for an admin previewing the rider view", async () => {
    stubStorage();
    window.localStorage.setItem("domestique.viewAsRider", "true");
    const { useEffectiveAdmin } = await freshIdentity();
    const { result } = renderHook(() => useEffectiveAdmin(), { wrapper: wrapWith(config(true)) });

    expect(result.current).toBe(false);
  });

  it("is false for a non-admin regardless of the stored preview choice", async () => {
    stubStorage();
    const { useEffectiveAdmin } = await freshIdentity();
    const { result } = renderHook(() => useEffectiveAdmin(), { wrapper: wrapWith(config(false)) });

    expect(result.current).toBe(false);
  });

  // A still-loading identity must not read as admin: that would flash an
  // admin-only control at every reader for a moment.
  it("is false while identity has not loaded yet", async () => {
    stubStorage();
    const { useEffectiveAdmin } = await freshIdentity();
    const { result } = renderHook(() => useEffectiveAdmin(), { wrapper: wrapWith() });

    expect(result.current).toBe(false);
  });
});

describe("useViewAsRider", () => {
  beforeEach(() => {
    stubStorage();
  });

  it("persists the choice to localStorage", async () => {
    const { useViewAsRider } = await freshIdentity();
    const { result } = renderHook(() => useViewAsRider());

    expect(result.current[0]).toBe(false);

    act(() => result.current[1](true));

    expect(result.current[0]).toBe(true);
    expect(window.localStorage.getItem("domestique.viewAsRider")).toBe("true");
  });

  // Every consumer shares one screen, so a flip through one has to reach the
  // others without either of them remounting.
  it("updates every consumer, not just the one that flipped it", async () => {
    const { useViewAsRider } = await freshIdentity();
    const a = renderHook(() => useViewAsRider());
    const b = renderHook(() => useViewAsRider());

    act(() => a.result.current[1](true));

    expect(a.result.current[0]).toBe(true);
    expect(b.result.current[0]).toBe(true);
  });

  // A storage that throws on every call must not make the toggle inert: the
  // choice still has to flip for as long as the page stays open.
  it("still flips for the session when localStorage throws", async () => {
    vi.stubGlobal("localStorage", {
      getItem: () => {
        throw new Error("storage disabled");
      },
      setItem: () => {
        throw new Error("storage disabled");
      },
    });
    const { useViewAsRider } = await freshIdentity();
    const { result } = renderHook(() => useViewAsRider());

    // Flips both ways so the assertion cannot pass on leftover module state
    // from an earlier test — only a live update through the throwing setter
    // explains ending up back at true.
    act(() => result.current[1](false));
    act(() => result.current[1](true));

    expect(result.current[0]).toBe(true);
  });

  // A quota-full or write-disabled browser must still let the toggle flip
  // in-session: only persistence is lost, not the live value every consumer
  // reads back through useSyncExternalStore.
  it("still flips in-session when getItem works but setItem throws", async () => {
    const entries = new Map<string, string>();
    vi.stubGlobal("localStorage", {
      getItem: (key: string) => entries.get(key) ?? null,
      setItem: () => {
        throw new Error("storage disabled");
      },
    });
    const { useViewAsRider } = await freshIdentity();
    const { result } = renderHook(() => useViewAsRider());

    act(() => result.current[1](true));

    expect(result.current[0]).toBe(true);
  });
});
