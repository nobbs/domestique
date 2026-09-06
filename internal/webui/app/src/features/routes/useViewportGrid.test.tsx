/**
 * The data contract every overlay leans on: `null` once switched off, not
 * whatever it last had.
 *
 * `keepPreviousData` carries a query's last result forward for any caller
 * with no live fetch behind it, disabled or not — verified here against the
 * real hook and a real `QueryClient`, since that behaviour is TanStack
 * Query's own and easy to assume rather than check.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import type { Bbox } from "../../lib/windGrid";

/** Bounds enough for `viewBbox` to compute a real bbox, and event registration for the effect. */
function fakeMap() {
  const listeners = new Map<string, Set<() => void>>();

  return {
    getBounds: () => ({
      getWest: () => 7,
      getSouth: () => 48,
      getEast: () => 8,
      getNorth: () => 49,
    }),
    on: (event: string, handler: () => void) => {
      const set = listeners.get(event) ?? new Set();
      set.add(handler);
      listeners.set(event, set);
    },
    off: (event: string, handler: () => void) => {
      listeners.get(event)?.delete(handler);
    },
  };
}

const map = fakeMap();

vi.mock("react-map-gl/maplibre", () => ({
  useMap: () => ({ current: map }),
}));

const { useViewportGrid } = await import("./useViewportGrid");

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });

  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

let seen: (number | null)[] = [];

function Harness({ on }: { on: boolean }) {
  const { data } = useViewportGrid("test-grid", on, (_bbox: Bbox) => Promise.resolve(1));
  seen.push(data);

  return null;
}

describe("useViewportGrid", () => {
  it("reports null once switched off, not the last grid it fetched", async () => {
    seen = [];
    const { rerender } = render(<Harness on={true} />, { wrapper });
    await vi.waitFor(() => expect(seen.at(-1)).toBe(1));

    rerender(<Harness on={false} />);
    await vi.waitFor(() => expect(seen.at(-1)).toBeNull());
  });
});
