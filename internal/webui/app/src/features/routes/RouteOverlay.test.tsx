/**
 * What the selected route draws over the library map, without a canvas.
 *
 * The overlay is not a map: it is the stack of sources and layers the entry
 * map mounts inside itself once a route is picked, so there is nothing here to
 * render but props. MapLibre's bindings are stood in for by fakes that record
 * what they were handed, which is how a question about the ink can be asked of
 * a canvas that never draws anything.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { weatherQuery } from "../../api/queries";
import type { Position, SurfaceRange, WeatherPoint } from "../../api/types";
import { CartographyProvider } from "../../components/map/CartographyContext";
import type { ForecastSample } from "../../lib/forecastSamples";
import { formatDistance, formatElevation } from "../../lib/format";
import type { MeasureKey } from "../../lib/measures";
import type { Profile } from "../../lib/profile";
import { buildProfile, buildWindowedProfile, cumulativeMetres, sampleAt } from "../../lib/profile";
import { summariseSurface } from "../../lib/surface";

interface LayerRecord {
  id: string;
  paint: Record<string, unknown>;
}

interface SourceRecord {
  id: string;
  data: { features?: unknown[] } | undefined;
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
  sources: [] as SourceRecord[],
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

vi.mock("../../lib/maplibre", () => ({}));

vi.mock("react-map-gl/maplibre", () => ({
  Layer: (props: LayerRecord) => {
    drawn.layers.push(props);

    return null;
  },
  Source: (props: SourceRecord & { children?: ReactNode }) => {
    drawn.sources.push({ id: props.id, data: props.data });

    return <>{props.children}</>;
  },
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
  drawn.sources = [];
  drawn.markers = [];
  drawn.projected = { x: 400, y: 300 };
  drawn.containerSize = { clientWidth: 800, clientHeight: 600 };
  drawn.mapReady = true;
});

/** A stage that climbs steadily, so its steepness has bands worth an edging. */
const CLIMBING: Position[] = Array.from(
  { length: 21 },
  (_, index): Position => [8 + index * 0.001, 49, index * 20],
);

/** A forecast reading, flat and unremarkable apart from the wind on it. */
function point(windDirectionDegrees: number): WeatherPoint {
  return {
    time: new Date().toISOString(),
    temperatureCelsius: 14,
    apparentTemperatureCelsius: 14,
    precipitationMillimetres: 0,
    precipitationProbabilityPercent: 0,
    windSpeedKmh: 22,
    windDirectionDegrees,
    weatherCode: 1,
    cloudCoverPercent: 20,
  };
}

/** Three readings spread evenly along a route, the shape a stage's forecast has. */
function samplesAlong(route: Position[]): ForecastSample[] {
  const distances = cumulativeMetres(route);
  const total = distances[distances.length - 1] ?? 0;

  return [0, 0.5, 1].map((share) => ({
    position: route[Math.round(share * (route.length - 1))] as Position,
    distanceMetres: share * total,
    arrivalAt: new Date(Date.now() + (1 + share) * 3_600_000),
  }));
}

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
    /** The forecast measure asked for, with a forecast seeded to answer it. */
    measure?: MeasureKey | null;
    withForecast?: boolean;
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
  // The position tooltip reads the forecast for its wind line, so the overlay
  // now needs a client in scope even where a test offers no samples at all.
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const samples = props.withForecast ? samplesAlong(coordinates) : [];
  if (props.withForecast) {
    // The wind blows from due east onto a road running due east, which is the
    // one reading whose colour a reader could not mistake for another.
    client.setQueryData(weatherQuery(samples).queryKey, { points: samples.map(() => point(90)) });
  }
  const jsx = (activeMetres: number | null) => (
    <QueryClientProvider client={client}>
      <CartographyProvider dark={props.darkBasemap ?? false}>
        <RouteOverlay
          coordinates={coordinates}
          samples={samples}
          measure={props.measure ?? null}
          surface={props.surface}
          surfaceSummary={surfaceSummary}
          profile={profile}
          activeProfile={props.activeProfile ?? profile}
          activeMetres={activeMetres}
          profileCollapsed={props.profileCollapsed ?? false}
          zoomWindow={props.zoomWindow ?? null}
          onZoomChange={onZoomChange}
        />
      </CartographyProvider>
    </QueryClientProvider>
  );
  const { rerender } = render(jsx(props.activeMetres ?? null));

  return {
    onZoomChange,
    profile,
    layer: (id: string) => drawn.layers.find((entry) => entry.id === id),
    ids: () => drawn.layers.map((entry) => entry.id),
    /** What one source was handed, for a layer that is mounted whether or not it draws. */
    features: (id: string) => drawn.sources.find((entry) => entry.id === id)?.data?.features ?? [],
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

/**
 * The route carries one encoding at a time. Steepness has the edging under the
 * casing until the reader asks what the wind is doing to them, and then the
 * wind has it — two ramps along one line would leave a colour belonging to
 * whichever of them the reader guessed.
 */
describe("the route tinted by the wind on the rider", () => {
  const WIND_LAYER = "route-wind-relation-line";

  it("tints the route once the wind is the measure asked for", () => {
    const view = show({ measure: "wind", withForecast: true });

    expect(view.layer(WIND_LAYER)).toBeDefined();
  });

  it("leaves the route alone for every other measure", () => {
    for (const measure of ["temperature", "rain", "cloud"] as const) {
      drawn.layers = [];
      drawn.sources = [];
      const view = show({ measure, withForecast: true });

      expect(view.layer(WIND_LAYER)).toBeUndefined();
    }
  });

  it("leaves the route alone with no measure asked for at all", () => {
    const view = show({ withForecast: true });

    expect(view.layer(WIND_LAYER)).toBeUndefined();
  });

  it("edges the steepness while nothing is tinting the route", () => {
    const view = show({ coordinates: CLIMBING });

    expect(view.features("route-gradient-4").length).toBeGreaterThan(0);
  });

  it("puts the steepness edging away for as long as the tint has the slot", () => {
    const view = show({ coordinates: CLIMBING, measure: "wind", withForecast: true });

    expect(view.layer(WIND_LAYER)).toBeDefined();
    // Still mounted, drawing nothing: a layer that came and went would be
    // re-added at whatever height the stack happened to have by then.
    for (const band of [1, 2, 3, 4]) {
      expect(view.layer(`route-gradient-${band}-line`)).toBeDefined();
      expect(view.features(`route-gradient-${band}`)).toEqual([]);
    }
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

    // How far along is the figure; how far is left is the bar under it, which
    // is why there is no second distance here to read.
    expect(screen.getByText(formatDistance(active.distanceMetres, "metric"))).toBeInTheDocument();
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

    expect(screen.getByText("0 m")).toBeInTheDocument();
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

  /*
   * The box is centred over the dot and sits above it, and the arrow is what
   * points at it. Where the pane's edge pushes the box sideways, the arrow
   * slides the other way by the same amount so it stays over the dot — which is
   * the whole reason it is not fixed to the middle. jsdom lays nothing out, so
   * the box keeps its default guessed width of 232 and the arithmetic below is
   * exact.
   */
  const HALF = 116;

  function arrowLeft(): string | undefined {
    const arrow = document.querySelector<HTMLElement>(".route-position-tooltip-arrow");

    return arrow?.style.left;
  }

  it("opens above the dot, centred on it, where there is room", () => {
    drawn.projected = { x: 400, y: 300 };
    const view = show({ activeMetres: ACTIVE_METRES });

    expect(view.marker()).toMatchObject({ anchor: "bottom", offset: [0, -10] });
    expect(arrowLeft()).toBe(`${HALF}px`);
  });

  it("opens below the dot when there is no room above it", () => {
    drawn.projected = { x: 400, y: 20 };
    const view = show({ activeMetres: ACTIVE_METRES });

    expect(view.marker()).toMatchObject({ anchor: "top", offset: [0, 10] });
  });

  it("slides back inside the pane near its left edge, and the arrow stays on the dot", () => {
    drawn.projected = { x: 100, y: 300 };
    const view = show({ activeMetres: ACTIVE_METRES });

    // Centred would hang the box 16px off the left edge, so it is pushed right.
    expect(view.marker()?.offset).toEqual([24, -10]);
    // 8px is where the box's left edge now is, so the dot is 92px along it.
    expect(arrowLeft()).toBe("92px");
  });

  it("slides back inside the pane near its right edge, the other way round", () => {
    drawn.projected = { x: 760, y: 300 };
    const view = show({ activeMetres: ACTIVE_METRES });

    expect(view.marker()?.offset).toEqual([-84, -10]);
    expect(arrowLeft()).toBe("200px");
  });

  /*
   * A pane narrower than the box itself: there is no position that both centres
   * the box and keeps it inside, so the clamp has nothing to satisfy. It must
   * still place the box rather than fold, and the arrow must stay within the
   * box's own corners rather than run off the end of it.
   */
  it("still places the box, and keeps the arrow inside it, in a pane narrower than itself", () => {
    drawn.containerSize = { clientWidth: 100, clientHeight: 600 };
    drawn.projected = { x: 60, y: 300 };
    const view = show({ activeMetres: ACTIVE_METRES });

    expect(view.marker()?.anchor).toBe("bottom");
    const left = Number.parseFloat(arrowLeft() ?? "");
    expect(left).toBeGreaterThanOrEqual(14);
    expect(left).toBeLessThanOrEqual(232 - 14);
  });

  it("draws nothing before the map instance has resolved", () => {
    drawn.mapReady = false;

    expect(() => show({ activeMetres: ACTIVE_METRES })).not.toThrow();
    expect(document.querySelector(".route-position-tooltip")).not.toBeInTheDocument();
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
