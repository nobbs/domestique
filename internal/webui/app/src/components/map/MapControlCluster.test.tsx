import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const stub = vi.hoisted(() => ({
  current: null as { getContainer: () => HTMLElement } | null,
}));

vi.mock("react-map-gl/maplibre", () => ({
  useMap: () => ({ current: stub.current }),
}));

const { MapControlCluster } = await import("./MapControlCluster");

afterEach(() => {
  stub.current = null;
});

describe("MapControlCluster", () => {
  it("uses MapLibre's lower-right control cluster", () => {
    const pane = document.createElement("div");
    const canvas = document.createElement("div");
    const cluster = document.createElement("div");
    cluster.className = "maplibregl-ctrl-bottom-right";
    canvas.append(cluster);
    pane.append(canvas);
    document.body.append(pane);
    stub.current = { getContainer: () => canvas };

    render(
      <MapControlCluster>
        <p>Furniture</p>
      </MapControlCluster>,
    );

    expect(cluster).toContainElement(screen.getByText("Furniture"));
    pane.remove();
  });

  it("falls back to the map pane before the control cluster exists", () => {
    const pane = document.createElement("div");
    const canvas = document.createElement("div");
    pane.append(canvas);
    document.body.append(pane);
    stub.current = { getContainer: () => canvas };

    render(
      <MapControlCluster>
        <p>Furniture</p>
      </MapControlCluster>,
    );

    expect(pane).toContainElement(screen.getByText("Furniture"));
    expect(canvas).not.toContainElement(screen.getByText("Furniture"));
    pane.remove();
  });
});
