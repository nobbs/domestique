import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { webUIConfigQuery } from "../api/queries";
import type { WebUIConfig } from "../api/types";
import { useEffectiveAdmin, useViewAsRider } from "./identity";

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

afterEach(() => {
  vi.unstubAllGlobals();
});

function config(admin: boolean): WebUIConfig {
  return {
    basemaps: [],
    sourceBaseUrls: {},
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
  it("is true for an admin with the rider view off", () => {
    stubStorage();
    const { result } = renderHook(() => useEffectiveAdmin(), { wrapper: wrapWith(config(true)) });

    expect(result.current).toBe(true);
  });

  it("is false for an admin previewing the rider view", () => {
    stubStorage();
    window.localStorage.setItem("domestique.viewAsRider", "true");
    const { result } = renderHook(() => useEffectiveAdmin(), { wrapper: wrapWith(config(true)) });

    expect(result.current).toBe(false);
  });

  it("is false for a non-admin regardless of the stored preview choice", () => {
    stubStorage();
    const { result } = renderHook(() => useEffectiveAdmin(), { wrapper: wrapWith(config(false)) });

    expect(result.current).toBe(false);
  });

  // A still-loading identity must not read as admin: that would flash an
  // admin-only control at every reader for a moment.
  it("is false while identity has not loaded yet", () => {
    stubStorage();
    const { result } = renderHook(() => useEffectiveAdmin(), { wrapper: wrapWith() });

    expect(result.current).toBe(false);
  });
});

describe("useViewAsRider", () => {
  it("persists the choice to localStorage", () => {
    stubStorage();
    const { result } = renderHook(() => useViewAsRider());

    expect(result.current[0]).toBe(false);

    act(() => result.current[1](true));

    expect(result.current[0]).toBe(true);
    expect(window.localStorage.getItem("domestique.viewAsRider")).toBe("true");
  });

  // Every consumer shares one screen, so a flip through one has to reach the
  // others without either of them remounting.
  it("updates every consumer, not just the one that flipped it", () => {
    stubStorage();
    const a = renderHook(() => useViewAsRider());
    const b = renderHook(() => useViewAsRider());

    act(() => a.result.current[1](true));

    expect(a.result.current[0]).toBe(true);
    expect(b.result.current[0]).toBe(true);
  });
});
