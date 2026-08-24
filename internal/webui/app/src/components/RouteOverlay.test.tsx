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
import type { Profile } from "../lib/profile";
import { buildProfile, buildWindowedProfile, sampleAt } from "../lib/profile";
import { summariseSurface } from "../lib/surface";

interface LayerRecord {
  id: string;
  paint: Record<string, unknown>;
}

interface MarkerRecord {
  anchor: string;
  offset: [number, number];
  longitude: number;
  latitude: number;
  className?: string | undefined;
}

const drawn = vi.hoisted(() => ({
  layers: [] as LayerRecord[],
  markers: [] as MarkerRecord[],
  // Where the tooltip's own hover point projects to on screen, so a test can
  // move it into whichever quadrant it wants to check the anchor flips into.
  projected: { x: 400, y: 300 },
  // The pane's own size, so a test can ask about a point with no room to
  // either side of it — a pane narrower than the tooltip itself, say.
  containerSize: { clientWidth: 800, clientHeight: 600 },
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
    drawn.markers.push({
      anchor: props.anchor,
      offset: props.offset,
      longitude: props.longitude,
      latitude: props.latitude,
      className: props.className,
    });

    return <>{props.children}</>;
  },
  // A camera the direction cues can ask what a pixel is worth on the ground,
  // and the tooltip can project a position against and read a pane size from.
  useMap: () => ({
    current: drawn.mapReady
      ? {
          getZoom: () => 13,
          getCenter: () => ({ lat: 49, lng: 8 }),
          getContainer: () => drawn.containerSize,
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
  drawn.containerSize = { clientWidth: 800, clientHeight: 600 };
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
    /** Defaults to the same whole-route profile `profile` builds, as an unzoomed chart would. */
    activeProfile?: Profile | null;
    profileCollapsed?: boolean;
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
  const jsx = (activeMetres: number | null) => (
    <RouteOverlay
      coordinates={coordinates}
      surface={props.surface}
      surfaceSummary={surfaceSummary}
      darkBasemap={props.darkBasemap ?? false}
      profile={profile}
      activeProfile={props.activeProfile ?? profile}
      activeMetres={activeMetres}
      profileCollapsed={props.profileCollapsed ?? false}
      zoomWindow={props.zoomWindow ?? null}
      onZoomChange={onZoomChange}
    />
  );
  const { rerender } = render(jsx(props.activeMetres ?? null));

  return {
    onZoomChange,
    profile,
    layer: (id: string) => drawn.layers.find((entry) => entry.id === id),
    ids: () => drawn.layers.map((entry) => entry.id),
    marker: () =>
      drawn.markers.find((marker) => marker.className === "route-position-tooltip-marker"),
    /** Re-renders the same tree with a different position, for a transition a fresh render cannot prove. */
    setActiveMetres: (metres: number | null) => rerender(jsx(metres)),
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
  it("uses a labelled start disc and a checkered finish flag", () => {
    show();

    expect(document.querySelector(".route-terminal--start")).toBeInTheDocument();
    expect(document.querySelector(".route-terminal--finish")).toBeInTheDocument();
  });

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
      screen.getByText(`${formatDistance(active.distanceMetres, "metric")} from start`),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        `${formatDistance(profile.endMetres - active.distanceMetres, "metric")} to end`,
      ),
    ).toBeInTheDocument();
    expect(screen.getByText(formatElevation(active.elevationMetres, "metric"))).toBeInTheDocument();
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

  it("is hidden from assistive technology while the profile's own readout is mounted", () => {
    show({ activeMetres: ACTIVE_METRES });

    const tooltip = document.querySelector(".route-position-tooltip");
    expect(tooltip).toHaveAttribute("aria-hidden", "true");
    expect(tooltip).not.toHaveAttribute("aria-live");
  });

  it("carries its own live announcement once the profile card is folded away", () => {
    show({ activeMetres: ACTIVE_METRES, profileCollapsed: true });

    const tooltip = document.querySelector(".route-position-tooltip");
    expect(tooltip).toHaveAttribute("aria-live", "polite");
    expect(tooltip).not.toHaveAttribute("aria-hidden");
  });

  it("is absent when there is no position to begin with", () => {
    show({ activeMetres: null });

    expect(document.querySelector(".route-position-tooltip")).not.toBeInTheDocument();
  });

  it("disappears once a shown position is cleared", () => {
    const view = show({ activeMetres: ACTIVE_METRES });
    expect(document.querySelector(".route-position-tooltip")).toBeInTheDocument();

    view.setActiveMetres(null);

    expect(document.querySelector(".route-position-tooltip")).not.toBeInTheDocument();
  });

  it("shows zero, not an em dash, for ground already ridden at the very start", () => {
    show({ activeMetres: 0 });

    expect(screen.getByText("0 m from start")).toBeInTheDocument();
  });

  it("shows zero, not an em dash, for ground left to ride at the very finish", () => {
    const whole = buildProfile(COORDINATES);
    expect(whole).not.toBeNull();
    if (!whole) {
      return;
    }

    show({ activeMetres: whole.endMetres });

    expect(screen.getByText("0 m to end")).toBeInTheDocument();
  });

  it("reads its sample from the profile the chart is displaying, not the whole route's", () => {
    // Elevation climbs steadily along the same geometry `COORDINATES` draws, so
    // its gradient and elevation at ACTIVE_METRES plainly differ from the flat
    // whole-route profile `show()` otherwise builds.
    const climbing = buildProfile(
      COORDINATES.map(
        ([longitude, latitude], index): Position => [longitude, latitude, index * 20],
      ),
    );
    expect(climbing).not.toBeNull();
    if (!climbing) {
      return;
    }
    const active = sampleAt(climbing, ACTIVE_METRES);
    expect(active).not.toBeNull();
    if (!active) {
      return;
    }

    show({ activeMetres: ACTIVE_METRES, activeProfile: climbing });

    expect(screen.getByText(formatElevation(active.elevationMetres, "metric"))).toBeInTheDocument();
    expect(screen.getByText(`${active.gradientPercent.toFixed(1)}%`)).toBeInTheDocument();
  });

  /*
   * A zoomed chart's own profile covers only the stretch it is showing, so a
   * hover on the dimmed route outside that stretch has no windowed sample to
   * give at all — the dot still has to move there, and the tooltip still has
   * to say something, both from the whole route rather than from a profile
   * that stops short of the position.
   */
  it("keeps the dot and the tooltip when the hover falls outside a zoom window", () => {
    const zoomed = buildWindowedProfile(COORDINATES, { startMetres: 1000, endMetres: 1400 });
    expect(zoomed).not.toBeNull();
    if (!zoomed) {
      return;
    }
    const OUTSIDE_WINDOW_METRES = 200;
    expect(sampleAt(zoomed, OUTSIDE_WINDOW_METRES)).toBeNull();
    const whole = buildProfile(COORDINATES);
    expect(whole).not.toBeNull();
    if (!whole) {
      return;
    }
    const wholeSample = sampleAt(whole, OUTSIDE_WINDOW_METRES);
    expect(wholeSample).not.toBeNull();
    if (!wholeSample) {
      return;
    }

    show({ activeMetres: OUTSIDE_WINDOW_METRES, activeProfile: zoomed });

    expect(document.querySelector(".route-position-tooltip")).toBeInTheDocument();
    expect(
      screen.getByText(formatElevation(wholeSample.elevationMetres, "metric")),
    ).toBeInTheDocument();
  });

  /*
   * The readout is silent in exactly this state — a zoomed chart has no
   * sample for ground outside its own window — so this is the one place
   * besides a folded card where the tooltip has to speak for itself.
   */
  it("announces itself when a hover outside the zoom window leaves the readout silent", () => {
    const zoomed = buildWindowedProfile(COORDINATES, { startMetres: 1000, endMetres: 1400 });
    expect(zoomed).not.toBeNull();
    if (!zoomed) {
      return;
    }

    show({ activeMetres: 200, activeProfile: zoomed, profileCollapsed: false });

    const tooltip = document.querySelector(".route-position-tooltip");
    expect(tooltip).toHaveAttribute("aria-live", "polite");
    expect(tooltip).not.toHaveAttribute("aria-hidden");
  });

  /*
   * A windowed profile interpolates its own coordinates independently of the
   * whole route's, so the two can disagree by enough to put the tooltip
   * beside the dot rather than on it. `activeProfile` is built here from a
   * geometry shifted a whole degree east of `COORDINATES`, so any mix-up
   * between the two samples is unmistakable rather than a rounding error.
   */
  it("positions its marker on the whole-route sample, not the one it is displaying", () => {
    const whole = buildProfile(COORDINATES);
    expect(whole).not.toBeNull();
    if (!whole) {
      return;
    }
    const wholeSample = sampleAt(whole, ACTIVE_METRES);
    expect(wholeSample).not.toBeNull();
    if (!wholeSample) {
      return;
    }
    const shifted = buildProfile(
      COORDINATES.map(
        ([longitude, latitude, elevation]): Position => [longitude + 1, latitude, elevation ?? 0],
      ),
    );
    expect(shifted).not.toBeNull();
    if (!shifted) {
      return;
    }

    const view = show({ activeMetres: ACTIVE_METRES, activeProfile: shifted });

    expect(view.marker()?.longitude).toBeCloseTo(wholeSample.longitude);
    expect(view.marker()?.longitude).not.toBeCloseTo(wholeSample.longitude + 1);
  });

  it("opens down and to the right from a point with room on every side", () => {
    drawn.projected = { x: 100, y: 100 };
    const view = show({ activeMetres: ACTIVE_METRES });

    expect(view.marker()).toMatchObject({ anchor: "top-left", offset: [14, 14] });
  });

  it("opens up and to the left once neither side near the bottom-right corner has room", () => {
    drawn.projected = { x: 780, y: 580 };
    const view = show({ activeMetres: ACTIVE_METRES });

    expect(view.marker()).toMatchObject({ anchor: "bottom-right", offset: [-14, -14] });
  });

  it("draws nothing before the map instance has resolved", () => {
    drawn.mapReady = false;

    expect(() => show({ activeMetres: ACTIVE_METRES })).not.toThrow();
    expect(document.querySelector(".route-position-tooltip")).not.toBeInTheDocument();
  });

  /*
   * A pane narrower than the tooltip itself, so neither side has the room the
   * box asks for. Both points sit in the same pane; only which one has more
   * room changes, which is what decides the side chosen.
   */
  it("opens toward whichever side has more room, when neither side fully fits", () => {
    drawn.containerSize = { clientWidth: 100, clientHeight: 600 };
    drawn.projected = { x: 60, y: 100 };
    const view = show({ activeMetres: ACTIVE_METRES });

    expect(view.marker()?.anchor).toBe("top-right");
  });

  it("opens toward the other side with more room, the other way round", () => {
    drawn.containerSize = { clientWidth: 100, clientHeight: 600 };
    drawn.projected = { x: 30, y: 100 };
    const view = show({ activeMetres: ACTIVE_METRES });

    expect(view.marker()?.anchor).toBe("top-left");
  });

  it("measures its own rendered box once mounted, replacing the default guess", () => {
    // jsdom lays nothing out, so the observed size is not asserted here — this
    // is the wiring proof that the effect subscribes, fires, and cleans up
    // without a real browser, which the anchor tests above cannot exercise.
    class StubResizeObserver {
      constructor(private readonly callback: ResizeObserverCallback) {}
      observe() {
        this.callback([], this as unknown as ResizeObserver);
      }
      unobserve() {}
      disconnect() {}
    }
    const original = globalThis.ResizeObserver;
    globalThis.ResizeObserver = StubResizeObserver as unknown as typeof ResizeObserver;

    try {
      expect(() => show({ activeMetres: ACTIVE_METRES })).not.toThrow();
    } finally {
      globalThis.ResizeObserver = original;
    }
  });
});
