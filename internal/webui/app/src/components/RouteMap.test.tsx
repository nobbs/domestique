/**
 * The map's interaction model, without a map.
 *
 * The component itself needs WebGL and so is not rendered in this suite. What is
 * tested here is everything around that: which mode the map is put in, what the
 * control does to it, and what one press of Escape leaves. MapLibre's bindings
 * are stood in for by a fake carrying exactly the surface those decisions touch,
 * which is how a question about the gestures can be asked of a canvas that never
 * draws anything.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BoundingBox, Position } from "../api/types";

/** The map the mocked bindings hand out, replaced before each render. */
const stub = vi.hoisted(() => ({ current: null as ReturnType<typeof fakeMap> | null }));

vi.mock("../lib/maplibre", () => ({}));

vi.mock("react-map-gl/maplibre", () => ({
  Map: ({ children }: { children?: ReactNode }) => <div data-testid="map">{children}</div>,
  Layer: () => null,
  NavigationControl: () => null,
  ScaleControl: () => null,
  Source: ({ children }: { children?: ReactNode }) => <>{children}</>,
  useMap: () => ({ current: stub.current }),
}));

const { RouteMap } = await import("./RouteMap");

/** How the camera was last framed, so a test can ask whether it flew there. */
interface Framing {
  duration: number;
}

function fakeMap() {
  const framings: Framing[] = [];
  const container = document.createElement("div");
  const canvasContainer = document.createElement("div");
  const canvas = document.createElement("canvas");
  canvas.tabIndex = 0;
  canvasContainer.append(canvas);
  container.append(canvasContainer);
  document.body.append(container);

  // Both start on, as MapLibre leaves them: reading the page is something this
  // component has to ask for, and a fake that started in that state would let a
  // component that never asked pass for one that did.
  let scrollZoom = true;
  let keyboard = true;
  const map = {
    getContainer: () => container,
    getCanvasContainer: () => canvasContainer,
    getCanvas: () => canvas,
    scrollZoom: {
      enable: () => {
        scrollZoom = true;
      },
      disable: () => {
        scrollZoom = false;
      },
    },
    keyboard: {
      enable: () => {
        keyboard = true;
      },
      disable: () => {
        keyboard = false;
      },
    },
  };

  return {
    exploring: () => scrollZoom && keyboard,
    // The reference react-map-gl hands to a child of the map. Only the parts
    // this component reaches for are here; the camera work is a no-op because
    // there is no camera.
    getMap: () => map,
    resize: () => {},
    // The camera does not move — there is none — but how it was asked to is the
    // whole of what the reduced-motion preference changes here.
    fitBounds: (_bounds: unknown, options: { duration: number }) => {
      framings.push({ duration: options.duration });
    },
    framings: () => framings,
    getContainer: () => container,
    // A camera the direction cues can ask what a pixel is worth on the ground.
    getZoom: () => 13,
    getCenter: () => ({ lat: 49, lng: 8 }),
    on: () => {},
    off: () => {},
  };
}

/** A closed square, ridden anticlockwise from its south-west corner. */
const LOOP: Position[] = [
  ...Array.from({ length: 20 }, (_, index): Position => [8 + index * 0.001, 49]),
  ...Array.from({ length: 20 }, (_, index): Position => [8.02, 49 + index * 0.0006]),
  ...Array.from({ length: 20 }, (_, index): Position => [8.02 - index * 0.001, 49.012]),
  ...Array.from({ length: 20 }, (_, index): Position => [8, 49.012 - index * 0.0006]),
  [8, 49],
];

const COORDINATES: Position[] = Array.from(
  { length: 21 },
  (_, index): Position => [8 + index * 0.001, 49, 100],
);
const BBOX: BoundingBox = [8, 49, 8.02, 49];

/**
 * A `matchMedia` answering one question: whether less movement was asked for.
 *
 * jsdom implements none, and the map reads the preference to decide whether to
 * fly to a new framing or simply be there.
 */
function stubReducedMotion(reduced: boolean) {
  vi.stubGlobal(
    "matchMedia",
    vi.fn((query: string) => ({
      matches: reduced && query.includes("prefers-reduced-motion"),
      addEventListener: () => {},
      removeEventListener: () => {},
    })),
  );
}

beforeEach(() => {
  stubReducedMotion(false);
  stub.current = fakeMap();
  // The map observes its container to keep the canvas sized to its pane, and
  // jsdom has no such observer of its own.
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
});

function show(
  props: {
    zoomWindow?: { startMetres: number; endMetres: number } | null;
    coordinates?: Position[];
  } = {},
) {
  const onZoomChange = vi.fn();
  // The credits under the map ask the style document what it wants attributed,
  // which is a query like any other. Nothing here is about that request, so the
  // client is left to fail it quietly rather than stubbed.
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <RouteMap
        styleUrl="https://tiles.example/style.json"
        coordinates={props.coordinates ?? COORDINATES}
        bbox={BBOX}
        title="Route 1"
        zoomWindow={props.zoomWindow ?? null}
        onZoomChange={onZoomChange}
      />
    </QueryClientProvider>,
  );

  return {
    onZoomChange,
    map: () => {
      const map = stub.current;
      if (!map) {
        throw new Error("the map was never created");
      }

      return map;
    },
    control: () => screen.getByRole("button", { name: "Explore map" }),
  };
}

describe("the map's camera", () => {
  it("flies to its framing by default", () => {
    const view = show();

    expect(view.map().framings()).toEqual([{ duration: 600 }]);
  });

  it("arrives at it outright when less movement was asked for", () => {
    stubReducedMotion(true);
    const view = show();

    // The camera is animated by MapLibre rather than by a CSS transition, so
    // the stylesheet's reduced-motion block cannot reach it and this is the
    // only place the preference can be honoured.
    expect(view.map().framings()).toEqual([{ duration: 0 }]);
  });
});

describe("the map's interaction model", () => {
  it("leaves the gestures to the page until they are asked for", () => {
    const view = show();

    expect(view.map().exploring()).toBe(false);
    expect(view.control()).toHaveAttribute("aria-pressed", "false");
  });

  it("hands them to the map when the control is pressed", async () => {
    const view = show();
    await userEvent.click(view.control());

    expect(view.map().exploring()).toBe(true);
    expect(view.control()).toHaveAttribute("aria-pressed", "true");
  });

  it("gives them back when it is pressed again", async () => {
    const view = show();
    await userEvent.click(view.control());
    await userEvent.click(view.control());

    expect(view.map().exploring()).toBe(false);
  });

  it("gives them back to Escape", async () => {
    const view = show();
    await userEvent.click(view.control());
    await userEvent.keyboard("{Escape}");

    expect(view.map().exploring()).toBe(false);
    expect(view.control()).toHaveAttribute("aria-pressed", "false");
  });

  it("leaves the stretch on show alone while it is the gestures being left", async () => {
    const view = show({ zoomWindow: { startMetres: 100, endMetres: 900 } });
    await userEvent.click(view.control());
    await userEvent.keyboard("{Escape}");

    expect(view.map().exploring()).toBe(false);
    expect(view.onZoomChange).not.toHaveBeenCalled();

    // And the next press, with nothing else held, is the way back to the route.
    await userEvent.keyboard("{Escape}");
    expect(view.onZoomChange).toHaveBeenCalledWith(null);
  });

  it("returns straight to the whole route when nothing is being explored", async () => {
    const view = show({ zoomWindow: { startMetres: 100, endMetres: 900 } });
    await userEvent.keyboard("{Escape}");

    expect(view.onZoomChange).toHaveBeenCalledWith(null);
  });
});

/**
 * The cues themselves are drawn into a canvas this suite never renders, so what
 * is asked here is the part that survives without one: the words a reader who is
 * not looking at the map has instead. They are the accessible equivalent of the
 * markers and arrows, so they are also the only place the component's reading of
 * the geometry is observable at all.
 */
describe("the map's start, finish, and direction cues", () => {
  it("says which way a point-to-point stage is ridden", () => {
    show();

    expect(
      screen.getByText(
        "Starts and finishes 1.5 km apart, the finish lying to the east. The ride leaves the start heading east.",
      ),
    ).toBeInTheDocument();
  });

  // The case the painted line cannot answer: both markers land on one point.
  it("says a loop comes back to where it started, and which way round", () => {
    show({ coordinates: LOOP });

    expect(screen.getByText(/Starts and finishes in the same place\./)).toBeInTheDocument();
    expect(screen.getByText(/leaves the start heading east/)).toBeInTheDocument();
    expect(screen.getByText(/returns from the north/)).toBeInTheDocument();
  });

  it("claims nothing about a stage that is not a ride", () => {
    show({ coordinates: [[8, 49]] });

    expect(screen.queryByText(/Starts and finishes/)).not.toBeInTheDocument();
  });
});
