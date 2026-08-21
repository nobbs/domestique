/**
 * What the selected route draws over the library map, without a canvas.
 *
 * The overlay is not a map: it is the stack of sources and layers the entry
 * map mounts inside itself once a route is picked, so there is nothing here to
 * render but props. MapLibre's bindings are stood in for by fakes that record
 * what they were handed, which is how a question about the ink can be asked of
 * a canvas that never draws anything.
 */

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Position, SurfaceRange } from "../api/types";

interface LayerRecord {
  id: string;
  paint: Record<string, unknown>;
}

const drawn = vi.hoisted(() => ({ layers: [] as LayerRecord[] }));

vi.mock("../lib/maplibre", () => ({}));

vi.mock("react-map-gl/maplibre", () => ({
  Layer: (props: LayerRecord) => {
    drawn.layers.push(props);

    return null;
  },
  Source: ({ children }: { children?: ReactNode }) => <>{children}</>,
  // A camera the direction cues can ask what a pixel is worth on the ground.
  useMap: () => ({
    current: {
      getZoom: () => 13,
      getCenter: () => ({ lat: 49, lng: 8 }),
      on: () => {},
      off: () => {},
    },
  }),
}));

const { RouteOverlay } = await import("./RouteOverlay");

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

beforeEach(() => {
  drawn.layers = [];
});

function show(
  props: {
    zoomWindow?: { startMetres: number; endMetres: number } | null;
    coordinates?: Position[];
    surface?: SurfaceRange[];
    darkBasemap?: boolean;
  } = {},
) {
  const onZoomChange = vi.fn();
  render(
    <RouteOverlay
      coordinates={props.coordinates ?? COORDINATES}
      surface={props.surface}
      darkBasemap={props.darkBasemap ?? false}
      zoomWindow={props.zoomWindow ?? null}
      onZoomChange={onZoomChange}
    />,
  );

  return {
    onZoomChange,
    layer: (id: string) => drawn.layers.find((entry) => entry.id === id),
    ids: () => drawn.layers.map((entry) => entry.id),
  };
}

describe("the route drawn over the library", () => {
  it("cases the line in the panel's own colour, so it reads as lifted off the ground", () => {
    const view = show();

    expect(view.layer("route-casing")?.paint["line-color"]).toBe("#fcfdff");
  });

  it("takes the dark casing when the cartography under it is dark", () => {
    const view = show({ darkBasemap: true });

    expect(view.layer("route-casing")?.paint["line-color"]).toBe("#24282c");
  });

  it("draws an unclassified route as one line", () => {
    const view = show();

    expect(view.layer("route-line")).toBeDefined();
    expect(view.ids().filter((id) => id.startsWith("route-surface-"))).toEqual([]);
  });

  it("draws a classified route as one line per class instead", () => {
    const view = show({
      surface: [
        { kind: "asphalt", startIndex: 0, endIndex: 10 },
        { kind: "gravel", startIndex: 10, endIndex: 20 },
      ],
    });

    expect(view.layer("route-line")).toBeUndefined();
    expect(view.ids().filter((id) => id.startsWith("route-surface-"))).toEqual([
      "route-surface-asphalt-line",
      "route-surface-gravel-line",
    ]);
  });
});

describe("the way back out of a stretch", () => {
  it("returns to the whole route on Escape", async () => {
    const view = show({ zoomWindow: { startMetres: 100, endMetres: 900 } });
    await userEvent.keyboard("{Escape}");

    expect(view.onZoomChange).toHaveBeenCalledWith(null);
  });

  it("leaves Escape to the page when the whole route is already shown", async () => {
    const view = show();
    await userEvent.keyboard("{Escape}");

    expect(view.onZoomChange).not.toHaveBeenCalled();
  });
});

/**
 * The cues themselves are drawn into a canvas this suite never renders, so what
 * is asked here is the part that survives without one: the words a reader who is
 * not looking at the map has instead. They are the accessible equivalent of the
 * markers and arrows, so they are also the only place the component's reading of
 * the geometry is observable at all.
 */
describe("the route's start, finish, and direction cues", () => {
  it("says which way a point-to-point route is ridden", () => {
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

  it("claims nothing about a route that is not a ride", () => {
    show({ coordinates: [[8, 49]] });

    expect(screen.queryByText(/Starts and finishes/)).not.toBeInTheDocument();
  });
});
