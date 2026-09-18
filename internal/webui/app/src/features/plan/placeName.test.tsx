import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

const reverse = vi.hoisted(() => vi.fn());
const placeNames = vi.hoisted(() => ({ value: true }));

vi.mock("../../api/generated", () => ({
  getReversePlaceQueryOptions: (
    params: { latitude: number; longitude: number },
    options: { query: { select: (response: unknown) => string } },
  ) => ({
    queryKey: ["place", params.latitude, params.longitude],
    queryFn: () => reverse(params),
    select: options.query.select,
  }),
}));
vi.mock("../../api/queries", () => ({
  webUIConfigQuery: () => ({
    queryKey: ["config"],
    queryFn: () => ({ placeNames: placeNames.value }),
  }),
}));

import { usePlaceName } from "./placeName";

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });

  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe("usePlaceName", () => {
  it("names the place the service answers with", async () => {
    placeNames.value = true;
    reverse.mockResolvedValue({ data: { name: "Kaiserstraße 12, Karlsruhe" } });

    const { result } = renderHook(() => usePlaceName(49.0094, 8.4044), { wrapper });

    await waitFor(() => expect(result.current).toBe("Kaiserstraße 12, Karlsruhe"));
  });

  it("asks about the rounded coordinate, so one place is one request", async () => {
    placeNames.value = true;
    reverse.mockResolvedValue({ data: {} });

    renderHook(() => usePlaceName(48.99881234, 8.47612345), { wrapper });

    await waitFor(() =>
      expect(reverse).toHaveBeenCalledWith({ latitude: 48.9988, longitude: 8.4761 }),
    );
  });

  it("says nothing where the place has no name", async () => {
    placeNames.value = true;
    reverse.mockResolvedValue({ data: {} });

    const { result } = renderHook(() => usePlaceName(30, -40), { wrapper });

    await waitFor(() => expect(reverse).toHaveBeenCalled());
    expect(result.current).toBe("");
  });

  it("asks nothing where no geocoder is configured", async () => {
    placeNames.value = false;
    reverse.mockClear();

    const { result } = renderHook(() => usePlaceName(49, 8), { wrapper });

    await waitFor(() => expect(result.current).toBe(""));
    expect(reverse).not.toHaveBeenCalled();
  });
});
