import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useStartupLocation } from "./startupLocation";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("useStartupLocation", () => {
  it("resolves to the coordinates the browser answers with", async () => {
    vi.stubGlobal("navigator", {
      geolocation: {
        getCurrentPosition: (found: (position: { coords: GeolocationCoordinates }) => void) =>
          found({ coords: { latitude: 49, longitude: 8 } as GeolocationCoordinates }),
      },
    });

    const { result } = renderHook(() => useStartupLocation(true));

    await waitFor(() => expect(result.current).toEqual([8, 49]));
  });

  it("stays null when the browser refuses, times out, or errors", () => {
    vi.stubGlobal("navigator", {
      geolocation: {
        getCurrentPosition: (_: unknown, denied: (error: unknown) => void) => denied(new Error()),
      },
    });

    const { result } = renderHook(() => useStartupLocation(true));

    expect(result.current).toBeNull();
  });

  it("stays null where the platform has no geolocation at all", () => {
    vi.stubGlobal("navigator", {});

    const { result } = renderHook(() => useStartupLocation(true));

    expect(result.current).toBeNull();
  });

  it("never asks the platform while disabled", () => {
    const getCurrentPosition = vi.fn();
    vi.stubGlobal("navigator", { geolocation: { getCurrentPosition } });

    renderHook(() => useStartupLocation(false));

    expect(getCurrentPosition).not.toHaveBeenCalled();
  });

  it("ignores a position that arrives after the caller has unmounted", () => {
    let respond: ((position: { coords: GeolocationCoordinates }) => void) | null = null;
    vi.stubGlobal("navigator", {
      geolocation: {
        getCurrentPosition: (found: (position: { coords: GeolocationCoordinates }) => void) => {
          respond = found;
        },
      },
    });

    const { unmount } = renderHook(() => useStartupLocation(true));
    unmount();

    expect(() =>
      respond?.({ coords: { latitude: 49, longitude: 8 } as GeolocationCoordinates }),
    ).not.toThrow();
  });
});
