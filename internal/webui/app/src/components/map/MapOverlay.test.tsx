import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const stub = vi.hoisted(() => ({
  current: null as { getContainer: () => HTMLElement } | null,
}));

vi.mock("react-map-gl/maplibre", () => ({
  useMap: () => ({ current: stub.current }),
}));

const { MapOverlay } = await import("./MapOverlay");

afterEach(() => {
  stub.current = null;
});

describe("MapOverlay", () => {
  it("waits for the map", () => {
    render(
      <MapOverlay>
        <p>Overlay</p>
      </MapOverlay>,
    );

    expect(screen.queryByText("Overlay")).not.toBeInTheDocument();
  });

  it("ports content beside the map canvas", () => {
    const pane = document.createElement("div");
    const canvas = document.createElement("div");
    pane.append(canvas);
    document.body.append(pane);
    stub.current = { getContainer: () => canvas };

    render(
      <MapOverlay>
        <p>Overlay</p>
      </MapOverlay>,
    );

    expect(pane).toContainElement(screen.getByText("Overlay"));
    expect(canvas).not.toContainElement(screen.getByText("Overlay"));
    pane.remove();
  });
});
