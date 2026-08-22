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
import { formatDistance, formatElevation } from "../lib/format";
import { buildProfile, sampleAt } from "../lib/profile";
import { summariseSurface } from "../lib/surface";

interface LayerRecord {
  id: string;
  paint: Record<string, unknown>;
}

interface MarkerRecord {
  anchor: string;
  offset: [number, number];
}

const drawn = vi.hoisted(() => ({
  layers: [] as LayerRecord[],
  markers: [] as MarkerRecord[],
  // Where the tooltip's own hover point projects to on screen, so a test can
  // move it into whichever quadrant it wants to check the anchor flips into.
  projected: { x: 400, y: 300 },
  // Whether the map instance has resolved yet, for the moment before it has.
  mapReady: true,
}));

vi.mock("../lib/maplibre", () => ({}));

vi.mock("react-map-gl/maplibre", () => ({
  Layer: (props: LayerRecord) => {
    drawn.layers.push(props);

    return null;
  },
  Source: ({ children }: { children?: ReactNode }) => <>{children}</>,
  // The content is covered above; what only a real map could place is the
  // corner it opens from, which this records so that can be asked about here
  // rather than left to the browser test alone.
  Marker: (props: MarkerRecord & { children?: ReactNode }) => {
    drawn.markers.push({ anchor: props.anchor, offset: props.offset });

    return <>{props.children}</>;
  },
  // A camera the direction cues can ask what a pixel is worth on the ground,
  // and the tooltip can project a position against and read a pane size from.
  useMap: () => ({
    current: drawn.mapReady
      ? {
          getZoom: () => 13,
          getCenter: () => ({ lat: 49, lng: 8 }),
          getContainer: () => ({ clientWidth: 800, clientHeight: 600 }),
          project: () => drawn.projected,
          on: () => {},
          off: () => {},
          // The drag-to-zoom link's map, which a profile makes it wire up
          // whether or not a test ever drags anything.
          getMap: () => ({
            getCanvasContainer: () => document.createElement("div"),
            project: () => ({ x: 400, y: 300 }),
            unproject: () => ({ lng: 8, lat: 49 }),
            dragPan: { enable: () => {}, disable: () => {}, isEnabled: () => true },
          }),
        }
      : undefined,
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
  drawn.markers = [];
  drawn.projected = { x: 400, y: 300 };
  drawn.mapReady = true;
});

function show(
  props: {
    zoomWindow?: { startMetres: number; endMetres: number } | null;
    coordinates?: Position[];
    surface?: SurfaceRange[];
    darkBasemap?: boolean;
    activeMetres?: number | null;
    withSurfaceSummary?: boolean;
  } = {},
) {
  const onZoomChange = vi.fn();
  const coordinates = props.coordinates ?? COORDINATES;
  // Built only when a test actually asks for a position: with one, the
  // drag-to-zoom link calls into `map.getMap()`, which this suite's fake map
  // has no reason to support otherwise.
  const profile = props.activeMetres != null ? buildProfile(coordinates) : null;
  const surfaceSummary =
    props.withSurfaceSummary && props.surface ? summariseSurface(coordinates, props.surface) : null;
  render(
    <RouteOverlay
      coordinates={coordinates}
      surface={props.surface}
      surfaceSummary={surfaceSummary}
      darkBasemap={props.darkBasemap ?? false}
      profile={profile}
      activeMetres={props.activeMetres ?? null}
      zoomWindow={props.zoomWindow ?? null}
      onZoomChange={onZoomChange}
    />,
  );

  return {
    onZoomChange,
    profile,
    layer: (id: string) => drawn.layers.find((entry) => entry.id === id),
    ids: () => drawn.layers.map((entry) => entry.id),
    marker: () => drawn.markers[0],
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

/**
 * The numbers on the dot: the tooltip a reader gets when the profile readout
 * below the map is not the thing being looked at.
 */
describe("the position tooltip", () => {
  const ACTIVE_METRES = 700;

  it("says distance from the start, distance to the end, elevation and gradient", () => {
    const view = show({
      surface: [{ kind: "asphalt", startIndex: 0, endIndex: 20 }],
      activeMetres: ACTIVE_METRES,
    });
    const profile = view.profile;
    expect(profile).not.toBeNull();
    if (!profile) {
      return;
    }
    const active = sampleAt(profile, ACTIVE_METRES);
    expect(active).not.toBeNull();
    if (!active) {
      return;
    }

    expect(
      screen.getByText(`${formatDistance(active.distanceMetres)} from start`),
    ).toBeInTheDocument();
    expect(
      screen.getByText(`${formatDistance(profile.endMetres - active.distanceMetres)} to end`),
    ).toBeInTheDocument();
    expect(screen.getByText(formatElevation(active.elevationMetres))).toBeInTheDocument();
    expect(screen.getByText(`${active.gradientPercent.toFixed(1)}%`)).toBeInTheDocument();
  });

  it("names the surface class from the same summary the readout uses", () => {
    show({
      surface: [{ kind: "gravel", startIndex: 0, endIndex: 20 }],
      withSurfaceSummary: true,
      activeMetres: ACTIVE_METRES,
    });

    expect(screen.getByText("Gravel")).toBeInTheDocument();
  });

  it("shows the other four fields and no placeholder for an unclassified stage", () => {
    show({ activeMetres: ACTIVE_METRES });

    expect(screen.getByText(/from start/)).toBeInTheDocument();
    expect(screen.getByText(/to end/)).toBeInTheDocument();
    expect(screen.queryByText("—")).not.toBeInTheDocument();
    // No surface line at all, rather than an empty one.
    for (const label of ["Asphalt", "Paving", "Compacted", "Gravel", "Ground", "Unsurveyed"]) {
      expect(screen.queryByText(label)).not.toBeInTheDocument();
    }
  });

  it("is hidden from assistive technology, since the readout already announces it", () => {
    show({ activeMetres: ACTIVE_METRES });

    expect(document.querySelector(".route-position-tooltip")).toHaveAttribute(
      "aria-hidden",
      "true",
    );
  });

  it("disappears once the position is cleared", () => {
    show({ activeMetres: null });

    expect(document.querySelector(".route-position-tooltip")).not.toBeInTheDocument();
  });

  it("opens down and to the right from a point near the top-left of the pane", () => {
    drawn.projected = { x: 100, y: 100 };
    const view = show({ activeMetres: ACTIVE_METRES });

    expect(view.marker()).toEqual({ anchor: "top-left", offset: [14, 14] });
  });

  it("opens up and to the left from a point near the bottom-right of the pane", () => {
    drawn.projected = { x: 700, y: 500 };
    const view = show({ activeMetres: ACTIVE_METRES });

    expect(view.marker()).toEqual({ anchor: "bottom-right", offset: [-14, -14] });
  });

  it("draws nothing before the map instance has resolved", () => {
    drawn.mapReady = false;

    expect(() => show({ activeMetres: ACTIVE_METRES })).not.toThrow();
    expect(document.querySelector(".route-position-tooltip")).not.toBeInTheDocument();
  });
});
