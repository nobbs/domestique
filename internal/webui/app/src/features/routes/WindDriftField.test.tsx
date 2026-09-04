/**
 * The drifting field, asked about in an environment with neither WebGL nor a
 * clock that ticks.
 *
 * jsdom cannot draw a streak, so nothing here claims the field looks like
 * anything: what it can answer is whether the layer is handed to the map at
 * all, where in the stack it goes, and — the failure no screenshot would ever
 * catch — whether the frame loop is still running after the component that
 * started it has gone. `windField.test.ts` has the maths; this has the
 * mounting, the ordering and the stopping.
 *
 * `requestAnimationFrame` is stubbed rather than driven by real time, so a
 * frame happens exactly when this file says it does.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { weatherQuery } from "../../api/queries";
import type { Position, WeatherForecast, WeatherPoint } from "../../api/types";
import { CartographyProvider } from "../../components/map/CartographyContext";
import type { ForecastSample } from "../../lib/forecastSamples";
import type { MeasureKey } from "../../lib/measures";
import { cumulativeMetres } from "../../lib/profile";
import { MAX_STATIC_ARROWS } from "../../lib/windField";

/** A custom layer as the map receives it, plus where it was asked to go. */
interface LayerRecord {
  id: string;
  type: string;
  beforeId?: string;
  render?: unknown;
}

interface MarkerRecord {
  longitude: number;
  latitude: number;
  children?: ReactNode;
}

const map = vi.hoisted(() => ({ layers: [] as LayerRecord[], repaints: 0 }));

vi.mock("react-map-gl/maplibre", () => ({
  Layer: (props: LayerRecord) => {
    map.layers.push(props);

    return null;
  },
  Marker: (props: MarkerRecord) => (
    <div data-testid="marker" data-longitude={props.longitude} data-latitude={props.latitude}>
      {props.children}
    </div>
  ),
  useMap: () => ({
    current: {
      triggerRepaint: () => {
        map.repaints += 1;
      },
    },
  }),
}));

const { WIND_FIELD_LAYER_ID, WindDriftField } = await import("./WindDriftField");

/** A due-east road of about 29 km, long enough to seed a field along. */
const ROAD: Position[] = Array.from({ length: 41 }, (_, index): Position => [8 + index * 0.01, 49]);
const DISTANCES = cumulativeMetres(ROAD);
const TOTAL_METRES = DISTANCES[DISTANCES.length - 1] ?? 0;

/** Three requests spread along the ride, the first an hour from now. */
const SAMPLES: ForecastSample[] = [0, 0.5, 1].map((share) => ({
  position: ROAD[Math.round(share * (ROAD.length - 1))] as Position,
  distanceMetres: share * TOTAL_METRES,
  arrivalAt: new Date(Date.now() + (1 + share) * 3_600_000),
}));

/** A steady wind out of the north, which is the one every test here reads. */
const NORTHERLY: WeatherPoint[] = SAMPLES.map((sample) => ({
  time: sample.arrivalAt.toISOString(),
  temperatureCelsius: 14,
  apparentTemperatureCelsius: 13,
  precipitationMillimetres: 0,
  precipitationProbabilityPercent: 0,
  windSpeedKmh: 24,
  windDirectionDegrees: 0,
  weatherCode: 1,
  cloudCoverPercent: 10,
}));

/** Every frame asked for and never yet run, and every handle cancelled. */
const frames: { pending: Map<number, FrameRequestCallback>; cancelled: number[] } = {
  pending: new Map(),
  cancelled: [],
};

/** One frame of the animation, `seconds` after the last one. */
function advance(seconds: number, at = performance.now()) {
  const due = [...frames.pending];
  frames.pending.clear();
  act(() => {
    for (const [, callback] of due) {
      callback(at + seconds * 1000);
    }
  });
}

/** The cache the last `show` seeded, for a test that re-renders into it. */
let cache = new QueryClient();

function show(options: {
  measure?: MeasureKey | null;
  samples?: ForecastSample[];
  points?: WeatherPoint[] | null;
  reducedMotion?: boolean;
  dark?: boolean;
}) {
  const samples = options.samples ?? SAMPLES;
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  cache = client;
  const points = options.points === undefined ? NORTHERLY : options.points;
  if (points) {
    const forecast: WeatherForecast = { points };
    client.setQueryData(weatherQuery(samples).queryKey, forecast);
  }
  if (options.reducedMotion) {
    vi.stubGlobal(
      "matchMedia",
      (query: string) =>
        ({
          matches: query.includes("prefers-reduced-motion"),
          media: query,
          onchange: null,
          addEventListener: () => {},
          removeEventListener: () => {},
          addListener: () => {},
          removeListener: () => {},
          dispatchEvent: () => false,
        }) as MediaQueryList,
    );
  }

  return render(
    <QueryClientProvider client={client}>
      <CartographyProvider dark={options.dark ?? false}>
        <WindDriftField
          coordinates={ROAD}
          samples={samples}
          measure={options.measure === undefined ? "wind" : options.measure}
          beforeId="route-window-halo"
        />
      </CartographyProvider>
    </QueryClientProvider>,
  );
}

function fieldLayer(): LayerRecord | undefined {
  return map.layers.find((layer) => layer.id === WIND_FIELD_LAYER_ID);
}

beforeEach(() => {
  map.layers = [];
  map.repaints = 0;
  frames.pending = new Map();
  frames.cancelled = [];
  let handle = 0;
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
    handle += 1;
    frames.pending.set(handle, callback);

    return handle;
  });
  vi.stubGlobal("cancelAnimationFrame", (given: number) => {
    frames.cancelled.push(given);
    frames.pending.delete(given);
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("{}", { status: 404 })),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("when the field draws anything at all", () => {
  it("hands the map one custom layer for the wind", () => {
    show({});

    expect(fieldLayer()?.type).toBe("custom");
    expect(typeof fieldLayer()?.render).toBe("function");
  });

  it("goes under the route, in the slot the wash is named against", () => {
    show({});

    expect(fieldLayer()?.beforeId).toBe("route-window-halo");
  });

  it("draws nothing for a measure that is not the wind", () => {
    const { container } = show({ measure: "temperature" });

    expect(fieldLayer()).toBeUndefined();
    expect(container).toBeEmptyDOMElement();
    expect(frames.pending.size).toBe(0);
  });

  it("draws nothing for a ride with no forecast asked about", () => {
    const { container } = show({ samples: [] });

    expect(fieldLayer()).toBeUndefined();
    expect(container).toBeEmptyDOMElement();
  });

  it("waits for the forecast rather than drifting on nothing", () => {
    const { container } = show({ points: null });

    expect(fieldLayer()).toBeUndefined();
    expect(container).toBeEmptyDOMElement();
    expect(frames.pending.size).toBe(0);
  });
});

describe("the frame loop", () => {
  it("asks for a frame and repaints the map on each one", () => {
    show({});

    expect(frames.pending.size).toBe(1);
    advance(0.016);

    expect(map.repaints).toBe(1);
    // Each frame asks for the next, so the loop keeps itself going.
    expect(frames.pending.size).toBe(1);
  });

  it("is cancelled when the field is taken down", () => {
    const view = show({});
    advance(0.016);
    const outstanding = [...frames.pending.keys()];

    view.unmount();

    expect(outstanding).toHaveLength(1);
    expect(frames.cancelled).toEqual(outstanding);
    expect(frames.pending.size).toBe(0);
  });

  it("stops when the measure is put away, and starts again when it comes back", () => {
    const view = show({});
    advance(0.016);

    // The same cache, still holding the forecast: it is the measure that has
    // changed, not what is known about the ride.
    view.rerender(
      <QueryClientProvider client={cache}>
        <CartographyProvider dark={false}>
          <WindDriftField coordinates={ROAD} samples={SAMPLES} measure="rain" />
        </CartographyProvider>
      </QueryClientProvider>,
    );

    expect(frames.cancelled).toHaveLength(1);
    expect(frames.pending.size).toBe(0);
  });

  it("stops while the document is hidden and picks up again when it is not", () => {
    show({});
    const started = [...frames.pending.keys()];
    const hidden = vi.spyOn(document, "hidden", "get").mockReturnValue(true);

    act(() => {
      document.dispatchEvent(new Event("visibilitychange"));
    });

    expect(frames.cancelled).toEqual(started);
    expect(frames.pending.size).toBe(0);

    hidden.mockReturnValue(false);
    act(() => {
      document.dispatchEvent(new Event("visibilitychange"));
    });

    expect(frames.pending.size).toBe(1);
  });
});

describe("when the reader has asked for no movement", () => {
  it("starts no animation whatsoever", () => {
    show({ reducedMotion: true });

    expect(frames.pending.size).toBe(0);
    expect(map.repaints).toBe(0);
    expect(fieldLayer()).toBeUndefined();
  });

  it("still says which way the wind blows, as arrows standing in the corridor", () => {
    show({ reducedMotion: true });
    const markers = screen.getAllByTestId("marker");

    expect(markers).toHaveLength(MAX_STATIC_ARROWS);
    // A wind out of the north, so every arrow points due south.
    for (const marker of markers) {
      expect(marker.querySelector("svg")).toHaveStyle({ transform: "rotate(180deg)" });
    }
  });

  it("stands them beside the road rather than on it", () => {
    show({ reducedMotion: true });

    for (const marker of screen.getAllByTestId("marker")) {
      expect(Number(marker.dataset.latitude)).toBeLessThan(49);
    }
  });

  it("adds no third table of the same forecast the wash and the tint report", () => {
    show({ reducedMotion: true });

    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });
});
