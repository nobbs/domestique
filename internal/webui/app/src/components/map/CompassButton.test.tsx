import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

const map = vi.hoisted(() => ({
  getBearing: vi.fn(() => 0),
  getPitch: vi.fn(() => 0),
  easeTo: vi.fn(),
  handlers: new Map<string, () => void>(),
  on(event: string, handler: () => void) {
    map.handlers.set(event, handler);
  },
  off(event: string) {
    map.handlers.delete(event);
  },
}));
const mapReady = vi.hoisted(() => ({ value: true }));

vi.mock("react-map-gl/maplibre", () => ({
  useMap: () => ({ current: mapReady.value ? map : null }),
}));

const { CompassButton } = await import("./CompassButton");

afterEach(() => {
  map.getBearing.mockReset();
  map.getBearing.mockReturnValue(0);
  map.getPitch.mockReset();
  map.getPitch.mockReturnValue(0);
  map.easeTo.mockReset();
  map.handlers.clear();
  mapReady.value = true;
});

describe("CompassButton", () => {
  it("turns the needle with the camera and eases back to north on a press", async () => {
    render(<CompassButton />);
    const button = screen.getByRole("button", { name: "Reset the view to north" });

    map.getBearing.mockReturnValue(45);
    act(() => map.handlers.get("rotate")?.());

    expect(button.querySelector("svg")).toHaveStyle({
      transform: "rotateX(0deg) rotateZ(-45deg)",
    });

    await userEvent.click(button);

    expect(map.easeTo).toHaveBeenCalledWith({ bearing: 0, pitch: 0, duration: 600 });
  });

  it("flattens the ring as the camera tilts", () => {
    render(<CompassButton />);

    map.getPitch.mockReturnValue(60);
    act(() => map.handlers.get("pitch")?.());

    expect(
      screen.getByRole("button", { name: "Reset the view to north" }).querySelector("svg"),
    ).toHaveStyle({ transform: "rotateX(60deg) rotateZ(0deg)" });
  });

  it("stops listening when unmounted", () => {
    const view = render(<CompassButton />);
    expect(map.handlers.has("rotate")).toBe(true);
    expect(map.handlers.has("pitch")).toBe(true);

    view.unmount();

    expect(map.handlers.has("rotate")).toBe(false);
    expect(map.handlers.has("pitch")).toBe(false);
  });

  it("is disabled while the map is unavailable", () => {
    mapReady.value = false;
    render(<CompassButton />);

    expect(screen.getByRole("button", { name: "Reset the view to north" })).toBeDisabled();
  });
});
