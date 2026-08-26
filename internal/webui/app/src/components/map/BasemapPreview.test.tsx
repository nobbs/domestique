import { render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const stub = vi.hoisted(() => ({
  current: null as { getContainer: () => HTMLElement } | null,
}));

vi.mock("react-map-gl/maplibre", () => ({
  useMap: () => ({ current: stub.current }),
  Map: () => null,
}));
vi.mock("../../lib/maplibre", () => ({}));

const { BasemapPreview } = await import("./BasemapPreview");

afterEach(() => {
  stub.current = null;
});

describe("BasemapPreview", () => {
  it("draws a picture of nothing rather than a thumbnail with no map to sit beside", () => {
    const { container } = render(
      <BasemapPreview styleUrl="https://tiles.example.test/style.json" selected={false} />,
    );

    expect(container.firstElementChild).toBeEmptyDOMElement();
  });

  it("wears the accent edge once it is the ground actually on screen", () => {
    stub.current = { getContainer: () => document.createElement("div") };
    const { container } = render(
      <BasemapPreview styleUrl="https://tiles.example.test/style.json" selected={true} />,
    );

    expect(container.firstElementChild).toHaveClass("ring-[var(--accent)]");
  });

  it("wears the ordinary edge for a basemap that is not on screen", () => {
    stub.current = { getContainer: () => document.createElement("div") };
    const { container } = render(
      <BasemapPreview styleUrl="https://tiles.example.test/style.json" selected={false} />,
    );

    expect(container.firstElementChild).toHaveClass("ring-[var(--rule)]");
  });
});
