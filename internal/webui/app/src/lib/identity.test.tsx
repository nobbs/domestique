import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { webUIConfigQuery } from "../api/queries";
import type { WebUIConfig } from "../api/types";
import { stubPendingFetch } from "../test/network";
import { useAdmin } from "./identity";

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
  } else {
    stubPendingFetch();
  }

  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
}

describe("useAdmin", () => {
  it("reads the admin claim", () => {
    expect(renderHook(() => useAdmin(), { wrapper: wrapWith(config(true)) }).result.current).toBe(
      true,
    );
    expect(renderHook(() => useAdmin(), { wrapper: wrapWith(config(false)) }).result.current).toBe(
      false,
    );
  });

  // A still-loading identity must not read as admin: that would flash an
  // admin-only control at every reader for a moment.
  it("is false while identity has not loaded yet", () => {
    expect(renderHook(() => useAdmin(), { wrapper: wrapWith() }).result.current).toBe(false);
  });
});
