/**
 * The clock a grid overlay reads its hour from, without a map or a network
 * to drive it.
 *
 * `useMap` is stubbed to a null map: what this answers has nothing to do with
 * the viewport or the fetch, only whether the hook re-renders on its own once
 * the wall clock crosses into a new hour, which nothing else does for it.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("react-map-gl/maplibre", () => ({
  useMap: () => ({ current: null }),
}));

const { useViewportGrid } = await import("./useViewportGrid");

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });

  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

let renders = 0;

function Harness({ on }: { on: boolean }) {
  renders += 1;
  useViewportGrid("test-grid", on, () => Promise.resolve(null));

  return null;
}

beforeEach(() => {
  renders = 0;
  vi.useFakeTimers();
  // A fixed moment shy of the hour, so advancing by a little over an hour is
  // guaranteed to cross exactly one boundary.
  vi.setSystemTime(new Date("2026-09-05T12:59:00Z"));
});

afterEach(() => {
  vi.useRealTimers();
});

describe("useViewportGrid", () => {
  it("re-renders on its own once the clock crosses into a new hour", () => {
    render(<Harness on={true} />, { wrapper });
    const before = renders;

    act(() => {
      vi.advanceTimersByTime(70 * 60_000);
    });

    expect(renders).toBeGreaterThan(before);
  });

  it("schedules nothing while switched off", () => {
    render(<Harness on={false} />, { wrapper });
    const before = renders;

    vi.advanceTimersByTime(70 * 60_000);

    expect(renders).toBe(before);
  });
});
