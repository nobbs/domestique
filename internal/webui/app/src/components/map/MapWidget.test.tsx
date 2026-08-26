import { act, render, screen } from "@testing-library/react";
import { type ReactNode, useEffect, useRef } from "react";
import { describe, expect, it, vi } from "vitest";

const drawn = vi.hoisted(() => ({
  maps: [] as Array<Record<string, unknown>>,
  onIdle: null as (() => void) | null,
}));

vi.mock("../../lib/maplibre", () => ({}));

vi.mock("react-map-gl/maplibre", () => ({
  Map: ({
    children,
    onLoad,
    onIdle,
    ...props
  }: {
    children?: ReactNode;
    onLoad?: (event: { target: { isStyleLoaded: () => boolean } }) => void;
    onIdle?: () => void;
  }) => {
    const initialOnLoad = useRef(onLoad);
    useEffect(() => initialOnLoad.current?.({ target: { isStyleLoaded: () => true } }), []);
    drawn.maps.push(props);
    drawn.onIdle = onIdle ?? null;

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

  it("waits for a replacement style to settle before mounting layers again", () => {
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

    act(() => drawn.onIdle?.());

    expect(screen.getByText("layered content")).toBeInTheDocument();
  });

  it("leaves the furniture standing while a replacement style loads", () => {
    const { rerender } = render(
      <MapWidget styleUrl="https://tiles.example/bright.json" furniture={<p>the controls</p>}>
        <p>layered content</p>
      </MapWidget>,
    );

    rerender(
      <MapWidget styleUrl="https://tiles.example/dark.json" furniture={<p>the controls</p>}>
        <p>layered content</p>
      </MapWidget>,
    );

    // The layers cannot outlive the style holding them and go; the furniture
    // owes the cartography nothing and has no reason to blink with them.
    expect(screen.queryByText("layered content")).not.toBeInTheDocument();
    expect(screen.getByText("the controls")).toBeInTheDocument();

    act(() => drawn.onIdle?.());

    expect(screen.getByText("layered content")).toBeInTheDocument();
    expect(screen.getByText("the controls")).toBeInTheDocument();
  });

  it("does not read readiness from an event that can describe the outgoing style", () => {
    // `styledata` fires several times on a basemap change, and the only one
    // that reports `isStyleLoaded()` arrives microseconds after the swap, while
    // the style being replaced is still the loaded one. Reading that as the new
    // style being ready remounted the layers early when it happened to fire,
    // and stranded them for good when it did not.
    render(
      <MapWidget styleUrl="https://tiles.example/style.json">
        <p>layered content</p>
      </MapWidget>,
    );

    expect(drawn.maps.at(-1)).not.toHaveProperty("onStyleData");
  });
});
