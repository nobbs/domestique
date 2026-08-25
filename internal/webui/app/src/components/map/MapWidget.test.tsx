import { act, render, screen } from "@testing-library/react";
import { type ReactNode, useEffect, useRef } from "react";
import { describe, expect, it, vi } from "vitest";

const drawn = vi.hoisted(() => ({
  maps: [] as Array<Record<string, unknown>>,
  onStyleData: null as ((event: { target: { isStyleLoaded: () => boolean } }) => void) | null,
}));

vi.mock("../../lib/maplibre", () => ({}));

vi.mock("react-map-gl/maplibre", () => ({
  Map: ({
    children,
    onLoad,
    onStyleData,
    ...props
  }: {
    children?: ReactNode;
    onLoad?: (event: { target: { isStyleLoaded: () => boolean } }) => void;
    onStyleData?: (event: { target: { isStyleLoaded: () => boolean } }) => void;
  }) => {
    const initialOnLoad = useRef(onLoad);
    useEffect(() => initialOnLoad.current?.({ target: { isStyleLoaded: () => true } }), []);
    drawn.maps.push(props);
    drawn.onStyleData = onStyleData ?? null;

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
    expect(drawn.maps.at(-1)).not.toHaveProperty("onError");
  });

  it("waits for a replacement style before mounting layers again", () => {
    const { rerender } = render(
      <MapWidget styleUrl="https://tiles.example/bright.json">
        <p>layered content</p>
      </MapWidget>,
    );

    rerender(
      <MapWidget styleUrl="https://tiles.example/dark.json">
        <p>layered content</p>
      </MapWidget>,
    );

    expect(screen.queryByText("layered content")).not.toBeInTheDocument();

    act(() => drawn.onStyleData?.({ target: { isStyleLoaded: () => true } }));

    expect(screen.getByText("layered content")).toBeInTheDocument();
  });
});
