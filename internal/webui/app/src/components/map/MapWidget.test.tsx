import { render, screen } from "@testing-library/react";
import { type ReactNode, useEffect } from "react";
import { describe, expect, it, vi } from "vitest";

const drawn = vi.hoisted(() => ({ maps: [] as Array<Record<string, unknown>> }));

vi.mock("../../lib/maplibre", () => ({}));

vi.mock("react-map-gl/maplibre", () => ({
  Map: ({ children, onLoad, ...props }: { children?: ReactNode; onLoad?: () => void }) => {
    useEffect(() => onLoad?.(), [onLoad]);
    drawn.maps.push(props);

    return <div data-testid="map">{children}</div>;
  },
}));

const { MapWidget } = await import("./MapWidget");

describe("MapWidget", () => {
  it("loads its style before mounting layers supplied by its caller", () => {
    render(
      <MapWidget styleUrl="https://tiles.example/style.json" ariaLabel="Example map">
        <p>layered content</p>
      </MapWidget>,
    );

    expect(screen.getByText("layered content")).toBeInTheDocument();
    expect(drawn.maps.at(-1)).toMatchObject({
      mapStyle: "https://tiles.example/style.json",
      "aria-label": "Example map",
      attributionControl: false,
    });
  });
});
