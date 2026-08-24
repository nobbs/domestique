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
import userEvent from "@testing-library/user-event";
import { forwardRef, type ReactNode, useImperativeHandle } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Basemap, BoundingBox, Position } from "../../api/types";

interface SourceRecord {
  id: string;
  data: {
    features: Array<{ geometry: { coordinates: Position[] }; properties: { key: string } }>;
  };
}

interface LayerRecord {
  id: string;
  paint: Record<string, unknown>;
  filter?: unknown;
}

const drawn = vi.hoisted(() => ({
  sources: [] as SourceRecord[],
  layers: [] as LayerRecord[],
  viewports: [] as Array<{ bounds: unknown; maxZoom: number }>,
  maps: [] as Array<Record<string, unknown>>,
  furniture: [] as Array<{ control: string; position: string }>,
  /** What the loaded map hands back as its own container. */
  container: null as HTMLElement | null,
  /** What the scale bar was told to report distance in. */
  scaleUnit: "" as string,
  map: {
    zoomIn: vi.fn(),
    zoomOut: vi.fn(),
    flyTo: vi.fn(),
    getZoom: vi.fn(() => 10),
  },
}));

vi.mock("../../lib/maplibre", () => ({}));

/*
 * The credit asks the style document what it must attribute, which is a fetch
 * and nothing this file is about. What is asked here is where it lands.
 */
vi.mock("../../components/MapCredits", () => ({
  MapCredits: () => <p data-testid="credits">© somebody</p>,
}));

/*
 * And the chooser decides nothing here either: what is asked of it in this file
 * is whether it is offered at all, and where it lands.
 */
vi.mock("../../components/BasemapPicker", () => ({
  BasemapPicker: ({ selectedName }: { selectedName: string }) => (
    <p data-testid="picker">{selectedName}</p>
  ),
}));

vi.mock("../../components/MapViewport", () => ({
  MapViewport: (props: { bounds: unknown; maxZoom: number }) => {
    drawn.viewports.push({ bounds: props.bounds, maxZoom: props.maxZoom });

    return null;
  },
}));

vi.mock("react-map-gl/maplibre", () => ({
  Map: forwardRef(function MapMock(
    {
      children,
      onLoad,
      onClick,
      onMouseMove,
      ...rest
    }: {
      children?: ReactNode;
      "aria-label"?: string;
      onLoad?: (event: { target: { getContainer: () => HTMLElement | null } }) => void;
      onClick?: (event: PointerEventStub) => void;
      onMouseMove?: (event: PointerEventStub) => void;
    },
    ref,
  ) {
    useImperativeHandle(ref, () => drawn.map);
    drawn.maps.push(rest);

    // A ref callback rather than the render pass: the real map reports itself
    // loaded once it is mounted, and so must the stand-in.
    return (
      <div
        data-testid="map"
        ref={(node) => {
          if (node) {
            onLoad?.({ target: { getContainer: () => drawn.container } });
          }
        }}
      >
        {/*
         * The pointer, without one: MapLibre hands its handlers whatever the
         * interactive layers had under the cursor, and these three stand for
         * the pointer arriving on a line, leaving it, and clicking there.
         */}
        <button type="button" onClick={() => onMouseMove?.(over("2/1"))}>
          point at a line
        </button>
        <button type="button" onClick={() => onMouseMove?.(over(null))}>
          point at the ground
        </button>
        <button type="button" onClick={() => onClick?.(over("2/1"))}>
          click a line
        </button>
        {children}
      </div>
    );
  }),
  Layer: (props: LayerRecord) => {
    drawn.layers.push(props);

    return null;
  },
  ScaleControl: ({ position, unit }: { position: string; unit: string }) => {
    drawn.furniture.push({ control: "scale", position });
    drawn.scaleUnit = unit;

    return null;
  },
  Marker: ({ children }: { children?: ReactNode }) => <>{children}</>,
  Source: ({ children, ...rest }: { children?: ReactNode } & SourceRecord) => {
    drawn.sources.push(rest);

    return <>{children}</>;
  },
}));

const { LibraryMap } = await import("./LibraryMap");

/** What MapLibre hands a pointer handler: the features under the cursor. */
interface PointerEventStub {
  features?: Array<{ properties?: Record<string, unknown> }>;
}

function over(key: string | null): PointerEventStub {
  return key === null ? { features: [] } : { features: [{ properties: { key } }] };
}

const BOUNDS: BoundingBox = [7.9, 48.9, 8.2, 49.1];

// The picker is mocked in this file, so its own gating on the list's length is
// untested here — that lives in BasemapPicker.test.tsx. What is real is the
// invariant the fixture should still honour: a chooser is only ever offered
// where there is a choice, so the tests below use a list of more than one.
const TWO_BASEMAPS: Basemap[] = [
  { name: "Streets", styleUrl: "https://tiles.example/style.json", darkCartography: false },
  { name: "Satellite", styleUrl: "https://imagery.example/style.json", darkCartography: true },
];

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

/** A map container with the corner MapLibre keeps its controls in. */
function containerWithCluster(): HTMLElement {
  const container = document.createElement("div");
  const cluster = document.createElement("div");
  cluster.className = "maplibregl-ctrl-bottom-left";
  container.append(cluster);
  document.body.append(container);

  return container;
}

beforeEach(() => {
  drawn.sources = [];
  drawn.layers = [];
  drawn.viewports = [];
  drawn.maps = [];
  drawn.furniture = [];
  drawn.map.zoomIn.mockReset();
  drawn.map.zoomOut.mockReset();
  drawn.map.flyTo.mockReset();
  drawn.map.getZoom.mockReset();
  drawn.map.getZoom.mockReturnValue(10);
  drawn.container?.remove();
  drawn.container = containerWithCluster();
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

  // The filter the selection layer draws by reads this, so a line without its
  // key is a line that can never be selected.
  it("carries each route's identity on the line itself", () => {
    show();

    expect(source("library-lines").data.features.map((feature) => feature.properties.key)).toEqual([
      "1/1",
      "2/1",
    ]);
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

  /*
   * The other half of dropping the library back: something has to be lifted out
   * of it. Without this the route the reader just picked fades along with the
   * rest, and the camera flies to a line that is no longer there to look at.
   */
  it("paints the route picked out of the column in the accent", () => {
    show({ selectedKey: "2/1" });

    expect(layer("library-selected-line").paint["line-color"]).toBe("#236fc7");
    expect(layer("library-selected-line").paint["line-opacity"]).toBe(1);
    expect(layer("library-selected-line").filter).toEqual(["==", ["get", "key"], "2/1"]);
  });

  it("takes the selection's ink from the basemap too", () => {
    show({ selectedKey: "2/1", darkBasemap: true });

    expect(layer("library-selected-line").paint["line-color"]).toBe("#70adfb");
  });

  // The opened route is drawn by the overlay's own stack, several times wider:
  // a second line underneath it would be paint nobody ever sees.
  it("leaves the accent to the overlay once the route is opened", () => {
    show({ selectedKey: "2/1", overlay: <div data-testid="overlay" /> });

    expect(drawn.layers.find((entry) => entry.id === "library-selected-line")).toBeUndefined();
  });

  it("draws nothing in the accent while no route is picked", () => {
    show({ selectedKey: null });

    expect(drawn.layers.find((entry) => entry.id === "library-selected-line")).toBeUndefined();
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
   * corner of its own. This map positions the sanitised credit itself instead.
   */
  it("draws the credit itself rather than letting MapLibre place it", () => {
    show();

    expect(drawn.maps.at(-1)).toMatchObject({
      "aria-label": "Map of the route library",
      attributionControl: false,
    });
  });

  /*
   * The app controls use the same icon set as the rest of the page, while
   * MapLibre still supplies the accurate scale bar.
   */
  it("renders Tabler map controls separately from the scale", async () => {
    show();

    expect(drawn.furniture.at(0)).toEqual({ control: "scale", position: "bottom-left" });
    expect(screen.getByRole("button", { name: "Find my location" })).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Zoom in" }));
    await userEvent.click(screen.getByRole("button", { name: "Zoom out" }));

    expect(drawn.map.zoomIn).toHaveBeenCalledOnce();
    expect(drawn.map.zoomOut).toHaveBeenCalledOnce();
  });

  it("keeps the credit out of the scale corner", () => {
    show();

    expect(drawn.container?.querySelector(".maplibregl-ctrl-bottom-left")?.textContent).toBe("");
    expect(screen.getByTestId("credits")).toBeInTheDocument();
  });

  /*
   * The chooser goes into the scale corner. Its absence cannot move the map
   * actions or attribution, because those use the right edge instead.
   */
  it("puts the basemap chooser in the scale cluster", () => {
    show({ basemaps: TWO_BASEMAPS, selectedBasemap: "Satellite", onBasemapChange: () => {} });

    const cluster = drawn.container?.querySelector(".maplibregl-ctrl-bottom-left");
    expect(cluster?.textContent).toBe("Satellite");
  });

  // Nothing is listening for a pick, so nothing is offered — the same bargain
  // the hit band strikes above.
  it("offers no chooser where no one is listening for one", () => {
    show();

    expect(screen.queryByTestId("picker")).not.toBeInTheDocument();
  });

  it("tells the scale bar which system to report distance in", () => {
    show({ unitSystem: "imperial" });

    expect(drawn.scaleUnit).toBe("imperial");
  });

  it("reports the scale bar in metric by default", () => {
    show();

    expect(drawn.scaleUnit).toBe("metric");
  });

  /*
   * Two pixels of ink is not a target. The band that is actually asked about is
   * far wider and invisible, and it carries the same identity as the line inside
   * it, so what is clicked and what lights up cannot disagree.
   */
  it("gives every route a band wide enough to point at", () => {
    show({ onPick: () => {} });

    const hit = layer("library-hit");
    expect(hit.paint["line-opacity"]).toBe(0);
    expect(hit.paint["line-width"]).toBeGreaterThan(12);
    expect(drawn.maps.at(-1)).toMatchObject({ interactiveLayerIds: ["library-hit"] });
  });

  // Nothing is listening, so nothing is offered: no band, no cursor promising a
  // click, and no paint answering a pointer.
  it("stays inert where no one is listening for a pick", () => {
    show();

    expect(drawn.layers.find((entry) => entry.id === "library-hit")).toBeUndefined();
    expect(drawn.maps.at(-1)).toMatchObject({ interactiveLayerIds: [] });
  });

  it("hands back the route the pointer clicked", async () => {
    const picked: string[] = [];
    show({ onPick: (key: string) => picked.push(key) });

    await userEvent.click(screen.getByRole("button", { name: "click a line" }));

    expect(picked).toEqual(["2/1"]);
  });

  /*
   * The map answers "this one?" before it is asked to commit: the line under the
   * pointer is lit in the accent a selection is drawn in, and the cursor says it
   * can be had.
   */
  it("lights the line under the pointer, and lets it go again", async () => {
    show({ onPick: () => {} });

    await userEvent.click(screen.getByRole("button", { name: "point at a line" }));

    expect(layer("library-hover-line").filter).toEqual(["==", ["get", "key"], "2/1"]);
    expect(layer("library-hover-line").paint["line-color"]).toBe("#236fc7");
    expect(drawn.maps.at(-1)).toMatchObject({ cursor: "pointer" });

    await userEvent.click(screen.getByRole("button", { name: "point at the ground" }));

    expect(drawn.maps.at(-1)).toMatchObject({ cursor: "" });
  });

  /*
   * It is painted in that accent already, and a second line over it at another
   * opacity would be the selection quietly changing colour under the pointer.
   * The cursor stays, because a second click on a picked route opens it.
   */
  it("leaves the route it is already showing unlit, but still offered", async () => {
    show({ selectedKey: "2/1", onPick: () => {} });

    await userEvent.click(screen.getByRole("button", { name: "point at a line" }));

    expect(drawn.layers.find((entry) => entry.id === "library-hover-line")).toBeUndefined();
    expect(drawn.maps.at(-1)).toMatchObject({ cursor: "pointer" });
  });

  // The cursor is a promise, and there is nothing left for a click on the open
  // route to do: it is the answer already.
  it("offers no cursor for the line a pick would do nothing to", async () => {
    show({ selectedKey: "2/1", inertKey: "2/1", onPick: () => {} });

    await userEvent.click(screen.getByRole("button", { name: "point at a line" }));

    expect(drawn.maps.at(-1)).toMatchObject({ cursor: "" });
  });

  /*
   * A licence obliges the credit to be visible, so a map that reports no corner
   * to put it in costs it its place in the cluster rather than its place on the
   * page.
   */
  it("keeps the credit on the page when the map reports no cluster", () => {
    drawn.container = document.createElement("div");
    show({ basemaps: TWO_BASEMAPS, selectedBasemap: "Streets", onBasemapChange: () => {} });

    expect(screen.getByTestId("credits")).toBeInTheDocument();
    expect(screen.getByTestId("picker")).toBeInTheDocument();
  });
});
