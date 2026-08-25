import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

const map = vi.hoisted(() => ({
  flyTo: vi.fn(),
  getZoom: vi.fn(() => 10),
  zoomIn: vi.fn(),
  zoomOut: vi.fn(),
}));
const mapReady = vi.hoisted(() => ({ value: true }));

vi.mock("react-map-gl/maplibre", () => ({
  Marker: ({ children }: { children?: React.ReactNode }) => <>{children}</>,
  useMap: () => ({ current: mapReady.value ? map : null }),
}));

const { MapControls } = await import("./MapControls");

afterEach(() => {
  map.flyTo.mockReset();
  map.getZoom.mockReset();
  map.getZoom.mockReturnValue(10);
  map.zoomIn.mockReset();
  map.zoomOut.mockReset();
  mapReady.value = true;
  vi.unstubAllGlobals();
});

describe("MapControls", () => {
  it("uses the loaded map for zoom and an available location", async () => {
    vi.stubGlobal("navigator", {
      geolocation: {
        getCurrentPosition: (
          found: (position: { coords: { latitude: number; longitude: number } }) => void,
        ) => found({ coords: { latitude: 49, longitude: 8 } }),
      },
    });
    render(<MapControls />);

    await userEvent.click(screen.getByRole("button", { name: "Zoom in" }));
    await userEvent.click(screen.getByRole("button", { name: "Zoom out" }));
    await userEvent.click(screen.getByRole("button", { name: "Find my location" }));

    expect(map.zoomIn).toHaveBeenCalledOnce();
    expect(map.zoomOut).toHaveBeenCalledOnce();
    expect(map.flyTo).toHaveBeenCalledWith({ center: [8, 49], zoom: 12 });
    expect(screen.getByRole("img", { name: "Your location" })).toBeInTheDocument();
  });

  it("does not read the map while it is unavailable", async () => {
    const getCurrentPosition = vi.fn();
    mapReady.value = false;
    vi.stubGlobal("navigator", { geolocation: { getCurrentPosition } });
    render(<MapControls />);

    await userEvent.click(screen.getByRole("button", { name: "Find my location" }));

    expect(getCurrentPosition).not.toHaveBeenCalled();
    expect(map.getZoom).not.toHaveBeenCalled();
  });
});
