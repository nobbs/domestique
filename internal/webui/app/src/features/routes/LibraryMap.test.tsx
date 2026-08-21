/**
 * What the entry map draws, without a canvas.
 *
 * MapLibre needs WebGL, so its bindings are stood in for by fakes that record
 * the sources and layers they were given. What is asked here is what the map is
 * made of — how many lines, in what ink, and what changes when a route is
 * picked — which is exactly what those props carry.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BoundingBox, Position } from "../../api/types";

interface SourceRecord {
  id: string;
  data: { features: Array<{ geometry: { coordinates: Position[] } }> };
}

interface LayerRecord {
  id: string;
  paint: Record<string, unknown>;
}

const drawn = vi.hoisted(() => ({
  sources: [] as SourceRecord[],
  layers: [] as LayerRecord[],
  viewports: [] as Array<{ bounds: unknown; maxZoom: number }>,
  maps: [] as Array<Record<string, unknown>>,
}));

vi.mock("../../lib/maplibre", () => ({}));

vi.mock("../../components/MapViewport", () => ({
  MapViewport: (props: { bounds: unknown; maxZoom: number }) => {
    drawn.viewports.push({ bounds: props.bounds, maxZoom: props.maxZoom });

    return null;
  },
}));

vi.mock("react-map-gl/maplibre", () => ({
  Map: ({ children, ...rest }: { children?: ReactNode; "aria-label"?: string }) => {
    drawn.maps.push(rest);

    return <div data-testid="map">{children}</div>;
  },
  Layer: (props: LayerRecord) => {
    drawn.layers.push(props);

    return null;
  },
  NavigationControl: () => null,
  ScaleControl: () => null,
  Source: ({ children, ...rest }: { children?: ReactNode } & SourceRecord) => {
    drawn.sources.push(rest);

    return <>{children}</>;
  },
  useMap: () => ({ current: null }),
}));

const { LibraryMap } = await import("./LibraryMap");

const BOUNDS: BoundingBox = [7.9, 48.9, 8.2, 49.1];

function line(key: string, offset = 0) {
  return {
    key,
    coordinates: [
      [8 + offset, 49],
      [8.05 + offset, 49.05],
      [8.1 + offset, 49.1],
    ] as Position[],
  };
}

beforeEach(() => {
  drawn.sources = [];
  drawn.layers = [];
  drawn.viewports = [];
  drawn.maps = [];
});

function show(props: Partial<Parameters<typeof LibraryMap>[0]> = {}) {
  // The credits under the map ask the style document what it wants attributed,
  // which is a query like any other and is nothing this file is about.
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });

  return render(
    <QueryClientProvider client={client}>
      <LibraryMap
        styleUrl="https://tiles.example/style.json"
        lines={[line("1/1"), line("2/1", 0.4)]}
        selectedKey={null}
        bounds={BOUNDS}
        {...props}
      />
    </QueryClientProvider>,
  );
}

function source(id: string) {
  const found = drawn.sources.find((entry) => entry.id === id);
  if (!found) {
    throw new Error(`expected a ${id} source`);
  }

  return found;
}

function layer(id: string) {
  const found = drawn.layers.find((entry) => entry.id === id);
  if (!found) {
    throw new Error(`expected a ${id} layer`);
  }

  return found;
}

describe("LibraryMap", () => {
  it("draws every route it was given as one line each", () => {
    show();

    expect(source("library-lines").data.features).toHaveLength(2);
  });

  /*
   * The whole decision the entry map rests on: one ink, one weight, nothing
   * classified. Forty-seven routes in five colours is a pattern rather than an
   * answer, and selection is the only thing that is allowed to stand out.
   */
  it("draws the library in one ink", () => {
    show({ selectedKey: null });

    expect(layer("library-line").paint["line-color"]).toBe("#1c2126");
    expect(layer("library-line").paint["line-opacity"]).toBe(0.68);
  });

  /*
   * The reader asked where one route goes, and the rest of the library is the
   * answer to *where* — so it drops back to context rather than being switched
   * off.
   */
  it("drops the rest of the library back once one route is the answer", () => {
    show({ selectedKey: "2/1" });

    expect(layer("library-line").paint["line-opacity"]).toBe(0.14);
  });

  // The selection is drawn over the library rather than cut out of it: the line
  // underneath is exactly covered by the overlay, and cutting would rebuild the
  // whole collection on every selection.
  it("leaves the selected route in the library beneath it", () => {
    show({ selectedKey: "2/1" });

    expect(source("library-lines").data.features).toHaveLength(2);
  });

  // The route itself is a stack of a dozen layers built from one route's
  // geometry, so it is the page's to hand over rather than this map's to draw.
  it("draws the selected route from the stack it was handed", () => {
    show({ selectedKey: "2/1", overlay: <div data-testid="overlay" /> });

    expect(screen.getByTestId("overlay")).toBeInTheDocument();
  });

  it("takes its ink from the basemap that is actually loaded", () => {
    show({ darkBasemap: true, selectedKey: null });

    expect(layer("library-line").paint["line-color"]).toBe("#eef0f3");
  });

  // A route whose geometry has one point is a point, and a line layer given one
  // draws nothing while still counting as a feature.
  it("leaves out geometry too short to be a line", () => {
    show({ lines: [{ key: "1/1", coordinates: [[8, 49]] }, line("2/1")] });

    expect(source("library-lines").data.features).toHaveLength(1);
  });

  it("hands the camera what it was told to frame", () => {
    show();

    expect(drawn.viewports.at(-1)).toEqual({ bounds: BOUNDS, maxZoom: 14 });
  });

  // A stretch of one route was asked for, so the camera is allowed closer than
  // it goes for the library or for a whole route.
  it("lets the camera go as close as it was told it may", () => {
    show({ maxZoom: 17 });

    expect(drawn.viewports.at(-1)).toEqual({ bounds: BOUNDS, maxZoom: 17 });
  });

  /*
   * MapLibre's own attribution control renders the provider's markup into a
   * corner of its own, and this map has one corner: the credit is drawn beneath
   * the zoom pair and the scale bar instead.
   */
  it("draws the credit itself rather than letting MapLibre place it", () => {
    show();

    expect(drawn.maps.at(-1)).toMatchObject({
      "aria-label": "Map of the route library",
      attributionControl: false,
    });
  });
});
