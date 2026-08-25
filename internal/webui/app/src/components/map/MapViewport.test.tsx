/**
 * The camera, without a map.
 *
 * MapLibre's bindings are stood in for by a fake carrying exactly the surface
 * this component touches — a container to observe and a `fitBounds` to record —
 * which is how a question about framing can be asked of a canvas that never
 * draws anything.
 */

import { render } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BoundingBox } from "../../api/types";
import type { Insets } from "../../lib/overlayInsets";
import { NO_INSETS } from "../../lib/overlayInsets";

const stub = vi.hoisted(() => ({ current: null as ReturnType<typeof fakeMap> | null }));

vi.mock("react-map-gl/maplibre", () => ({
  useMap: () => ({ current: stub.current }),
}));

const { MapViewport } = await import("./MapViewport");

interface Framing {
  bounds: unknown;
  options: { padding: Insets; duration: number; maxZoom: number };
}

function fakeMap() {
  const framings: Framing[] = [];
  const container = document.createElement("div");
  // jsdom lays nothing out, so the pane says how big it is itself: the padding
  // is held against the frame it is padding.
  Object.defineProperty(container, "clientWidth", { value: 1280 });
  Object.defineProperty(container, "clientHeight", { value: 800 });
  let resizes = 0;

  return {
    framings: () => framings,
    resizes: () => resizes,
    getContainer: () => container,
    resize: () => {
      resizes++;
    },
    fitBounds: (bounds: unknown, options: Framing["options"]) => {
      framings.push({ bounds, options });
    },
  };
}

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

const BOUNDS: BoundingBox = [7.9, 48.9, 8.2, 49.1];

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

function show(bounds: BoundingBox | null, padding?: number, insets?: Insets) {
  return render(
    <MapViewport
      bounds={bounds}
      maxZoom={14}
      {...(padding === undefined ? {} : { padding })}
      {...(insets === undefined ? {} : { insets })}
    />,
  );
}

/** The gutter alone, on every side: what an empty pane is framed with. */
function evenly(gutter: number): Insets {
  return { top: gutter, right: gutter, bottom: gutter, left: gutter };
}

function map() {
  const value = stub.current;
  if (!value) {
    throw new Error("expected a map");
  }

  return value;
}

describe("MapViewport", () => {
  it("frames what it was given, in the corner order MapLibre expects", () => {
    show(BOUNDS);

    expect(map().framings()).toEqual([
      {
        bounds: [
          [7.9, 48.9],
          [8.2, 49.1],
        ],
        options: { padding: evenly(56), duration: 600, maxZoom: 14 },
      },
    ]);
  });

  /*
   * The camera is animated by MapLibre rather than by a transition, so the
   * stylesheet's reduced-motion block cannot reach it. A reader who asked for
   * less movement is given the new framing outright.
   */
  it("arrives rather than flies when less movement was asked for", () => {
    stubReducedMotion(true);
    show(BOUNDS);

    expect(map().framings()[0]?.options.duration).toBe(0);
  });

  it("leaves the camera alone when there is nothing to frame", () => {
    show(null);

    expect(map().framings()).toEqual([]);
  });

  it("takes the room to leave around the bounds from its caller", () => {
    show(BOUNDS, 24);

    expect(map().framings()[0]?.options.padding).toEqual(evenly(24));
  });

  /*
   * The panels float over the map rather than beside it, so a route framed
   * against the whole pane opens half under the column beside it. The camera
   * frames it in what the reader can actually see instead.
   */
  it("holds the framing out from under the panels standing on the map", () => {
    show(BOUNDS, 56, { ...NO_INSETS, left: 424, bottom: 180 });

    expect(map().framings()[0]?.options.padding).toEqual({
      top: 56,
      right: 56,
      bottom: 236,
      left: 480,
    });
  });

  // A panel opening is a change of subject as much as a different route is: the
  // ground the reader was looking at has just gone under it.
  it("re-frames when a panel takes a side of the map", () => {
    const { rerender } = show(BOUNDS);
    rerender(<MapViewport bounds={BOUNDS} maxZoom={14} insets={{ ...NO_INSETS, left: 424 }} />);

    expect(map().framings()).toHaveLength(2);
  });

  // Measured on every layout, so the same measurement must not be a new one.
  it("does not re-frame for a fresh measurement that says the same thing", () => {
    const insets = { ...NO_INSETS, left: 424 };
    const { rerender } = show(BOUNDS, 56, insets);
    rerender(<MapViewport bounds={BOUNDS} maxZoom={14} padding={56} insets={{ ...insets }} />);

    expect(map().framings()).toHaveLength(1);
  });

  // The map mounts inside a pane whose height is not resolved on the first
  // paint, so the canvas is wrong until something tells it to look again.
  it("sizes the canvas to its pane on mount", () => {
    show(BOUNDS);

    expect(map().resizes()).toBeGreaterThan(0);
  });

  /*
   * The observer is an optimisation over the one sizing that has to happen, so a
   * runtime without it gets a map that never reflows rather than no map: throwing
   * here would take the whole canvas down with it.
   */
  it("still sizes the canvas where nothing can be observed", () => {
    vi.stubGlobal("ResizeObserver", undefined);

    expect(() => show(BOUNDS)).not.toThrow();
    expect(map().resizes()).toBeGreaterThan(0);
    expect(map().framings()).toHaveLength(1);
  });

  /*
   * Panning away to look at the surrounding roads costs nothing and needs no way
   * back: only a change of subject moves the camera.
   */
  it("does not re-frame for a render that changed nothing", () => {
    const { rerender } = show(BOUNDS);
    rerender(<MapViewport bounds={BOUNDS} maxZoom={14} />);

    expect(map().framings()).toHaveLength(1);
  });

  it("re-frames when the subject changes", () => {
    const { rerender } = show(BOUNDS);
    rerender(<MapViewport bounds={[8.0, 49.0, 8.1, 49.05]} maxZoom={14} />);

    expect(map().framings()).toHaveLength(2);
  });
});
