/**
 * The forecast wash, asked about without a canvas.
 *
 * The same approach `RouteOverlay.test.tsx` takes: MapLibre's bindings are
 * fakes that record what they were handed, so a question about the ink can be
 * asked of a map that never draws anything. The forecast itself is seeded into
 * the query cache under `weatherQuery`'s own key, as `ForecastStrip.test.tsx`
 * seeds it, with a catch-all 404 `fetch` underneath so an unseeded request is
 * loud rather than a silent hang.
 *
 * The question that matters most here is one about the geometry rather than
 * about the paint: no two shapes the wash emits may share any ground, because
 * two translucent shapes over the same ground blend into a colour that is in no
 * band and reads as weather nobody forecast.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { weatherQuery } from "../../api/queries";
import type { Position, WeatherForecast, WeatherPoint } from "../../api/types";
import { CartographyProvider } from "../../components/map/CartographyContext";
import { FADE_RINGS } from "../../lib/conditionsCorridor";
import type { ScalarSample } from "../../lib/conditionsField";
import type { ForecastSample } from "../../lib/forecastSamples";
import { MEASURES, type MeasureKey } from "../../lib/measures";
import { cumulativeMetres } from "../../lib/profile";

/** The geometry types `polygon-clipping` does not export at runtime. */
type Pair = [number, number];
type Ring = Pair[];
type MultiPolygon = Ring[][];

interface LayerRecord {
  id: string;
  type: string;
  beforeId?: string;
  paint: Record<string, unknown>;
}

interface SourceRecord {
  id: string;
  data: unknown;
}

/** One filled ring as the layer receives it: a shape, a colour and an alpha. */
interface WashFeature {
  geometry: { type: string; coordinates: MultiPolygon };
  properties: { colour: string; opacity: number };
}

const drawn = vi.hoisted(() => ({
  layers: [] as LayerRecord[],
  sources: [] as SourceRecord[],
}));

vi.mock("react-map-gl/maplibre", () => ({
  Layer: (props: LayerRecord) => {
    drawn.layers.push(props);

    return null;
  },
  Source: (props: SourceRecord & { children?: ReactNode }) => {
    drawn.sources.push({ id: props.id, data: props.data });

    return <>{props.children}</>;
  },
}));

const { BAND_SCAN_METRES, ConditionsWash, WASH_LAYER_ID, WASH_OPACITY, bandRuns } = await import(
  "./ConditionsWash"
);

/** A due-east road of about 29 km, long enough for a band to change along it. */
const ROAD: Position[] = Array.from({ length: 41 }, (_, index): Position => [8 + index * 0.01, 49]);

/**
 * The same road out and back, the two arms a kilometre apart — far tighter than
 * the corridor's own half-width, which is the shape a wide line folded over
 * itself on and drew a dark streak along.
 */
const HAIRPIN: Position[] = [
  ...Array.from({ length: 21 }, (_, index): Position => [8 + index * 0.01, 49]),
  ...Array.from({ length: 21 }, (_, index): Position => [8.2 - index * 0.01, 49.009]),
];

/**
 * A ride that comes back to where it started, 1.5 kilometres out — the shape
 * most rides are, and the one whose corridor has to come out with a hole in
 * it rather than a filled-in middle. Small and low-vertex on purpose: the
 * closed ring is what costs the clip, and the centre only has to clear the
 * 700 m corridor this test's forecast produces.
 */
const LOOP: Position[] = Array.from({ length: 15 }, (_, index): Position => {
  const angle = (2 * Math.PI * index) / 14;

  return [8 + 0.02054 * Math.cos(angle), 49 + 0.013474 * Math.sin(angle)];
});

const DISTANCES = cumulativeMetres(ROAD);
const TOTAL_METRES = DISTANCES[DISTANCES.length - 1] ?? 0;

/** A forecast point carrying one reading, the rest held flat and unremarkable. */
function point(overrides: Partial<WeatherPoint>): WeatherPoint {
  return {
    time: new Date().toISOString(),
    temperatureCelsius: 14,
    apparentTemperatureCelsius: 14,
    precipitationMillimetres: 0,
    precipitationProbabilityPercent: 0,
    windSpeedKmh: 5,
    windDirectionDegrees: 270,
    weatherCode: 1,
    cloudCoverPercent: 5,
    ...overrides,
  };
}

/** Three samples spread evenly along a route, the first `leadHours` from now. */
function samplesLeading(leadHours: number, route: Position[] = ROAD): ForecastSample[] {
  const distances = cumulativeMetres(route);
  const total = distances[distances.length - 1] ?? 0;

  return [0, 0.5, 1].map((share) => ({
    position: route[Math.round(share * (route.length - 1))] as Position,
    distanceMetres: share * total,
    arrivalAt: new Date(Date.now() + (leadHours + share) * 3_600_000),
  }));
}

function show(options: {
  measure?: MeasureKey | null;
  coordinates?: Position[];
  samples?: ForecastSample[];
  points?: WeatherPoint[];
  seed?: boolean;
  dark?: boolean;
}) {
  const coordinates = options.coordinates ?? ROAD;
  const samples = options.samples ?? samplesLeading(1, coordinates);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  if (options.seed !== false && options.points) {
    const forecast: WeatherForecast = { points: options.points };
    client.setQueryData(weatherQuery(samples).queryKey, forecast);
  }

  return render(
    <QueryClientProvider client={client}>
      <CartographyProvider dark={options.dark ?? false}>
        <ConditionsWash
          coordinates={coordinates}
          samples={samples}
          measure={options.measure === undefined ? "rain" : options.measure}
          beforeId="route-window-halo"
        />
      </CartographyProvider>
    </QueryClientProvider>,
  );
}

function washLayer(): LayerRecord | undefined {
  return drawn.layers.find((layer) => layer.id === WASH_LAYER_ID);
}

/** Every ring the wash handed the layer, in the order it emitted them. */
function washFeatures(): WashFeature[] {
  const data = drawn.sources[0]?.data as { features?: WashFeature[] } | undefined;

  return data?.features ?? [];
}

/** The north-south reach of everything the wash emitted, in degrees. */
function latitudeSpan(features: WashFeature[]): number {
  const latitudes = features.flatMap((feature) =>
    feature.geometry.coordinates.flat(2).map((point) => point[1]),
  );

  return latitudes.length > 0 ? Math.max(...latitudes) - Math.min(...latitudes) : 0;
}

/**
 * Whether a point falls inside one ring, by ray casting over every polygon and
 * hole it is made of.
 *
 * Hand-rolled rather than asked of `polygon-clipping`: its sweep line refuses
 * some of the very arrangements this test exists to examine, and an oracle that
 * throws on the interesting cases cannot report on them.
 */
function covers(feature: WashFeature, longitude: number, latitude: number): boolean {
  let inside = false;
  for (const polygon of feature.geometry.coordinates) {
    for (const ring of polygon) {
      for (let i = 0, j = ring.length - 1; i < ring.length; j = i++) {
        const [xi, yi] = ring[i] as [number, number];
        const [xj, yj] = ring[j] as [number, number];
        if (yi > latitude !== yj > latitude) {
          const at = ((xj - xi) * (latitude - yi)) / (yj - yi) + xi;
          if (longitude < at) {
            inside = !inside;
          }
        }
      }
    }
  }

  return inside;
}

/**
 * Ground that more than one ring paints, which is the whole thing this layer's
 * shape exists to prevent: a fragment painted twice blends twice and reads as a
 * streak that no forecast put there.
 *
 * Sampled on a grid over everything emitted rather than measured as an
 * intersection area, so the answer never depends on a boolean operation
 * completing.
 */
function overlapping(features: WashFeature[], steps = 90): string[] {
  const points = features.flatMap((feature) => feature.geometry.coordinates.flat(2)) as [
    number,
    number,
  ][];
  if (points.length === 0) {
    return [];
  }
  const longitudes = points.map(([longitude]) => longitude);
  const latitudes = points.map(([, latitude]) => latitude);
  const west = Math.min(...longitudes);
  const east = Math.max(...longitudes);
  const south = Math.min(...latitudes);
  const north = Math.max(...latitudes);

  const found: string[] = [];
  for (let row = 0; row <= steps; row++) {
    for (let column = 0; column <= steps; column++) {
      const longitude = west + ((east - west) * column) / steps;
      const latitude = south + ((north - south) * row) / steps;
      const hits = features.flatMap((feature, index) =>
        covers(feature, longitude, latitude) ? [index] : [],
      );
      if (hits.length > 1) {
        found.push(`${hits.join(" and ")} both paint ${longitude}, ${latitude}`);
      }
    }
  }

  return found;
}

const RAIN = MEASURES.find((measure) => measure.key === "rain") as (typeof MEASURES)[number];
const TEMPERATURE = MEASURES.find(
  (measure) => measure.key === "temperature",
) as (typeof MEASURES)[number];

/** Rain rising from nothing to a downpour: every band in order along one road. */
const RISING_RAIN = [0, 3, 8].map((millimetres) =>
  point({ precipitationMillimetres: millimetres }),
);

/** The same for temperature, whose every band paints something. */
const WARMING = [-5, 12, 30].map((celsius) => point({ apparentTemperatureCelsius: celsius }));

beforeEach(() => {
  drawn.layers = [];
  drawn.sources = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("{}", { status: 404 })),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("the ground the wash covers", () => {
  it("fills polygons rather than stroking a line", () => {
    show({ points: RISING_RAIN });

    expect(washLayer()?.type).toBe("fill");
    expect(washFeatures()[0]?.geometry.type).toBe("MultiPolygon");
  });

  it("sits under everything the route itself draws", () => {
    show({ points: RISING_RAIN });

    expect(washLayer()?.beforeId).toBe("route-window-halo");
  });

  /*
   * The bug this shape exists for. A translucent band eight kilometres wide
   * folds over itself wherever the route bends tighter than its own half-width,
   * and the fragments blend to a colour three times over — a dark streak the
   * reader has every reason to take for weather. Two filled rings that share no
   * ground cannot do that, whatever the route does.
   */
  it("never paints the same ground twice, even on a hairpin", () => {
    show({ measure: "temperature", coordinates: HAIRPIN, points: WARMING });
    const features = washFeatures();

    expect(features.length).toBeGreaterThan(1);
    expect(overlapping(features)).toEqual([]);
  });

  /* The rings being disjoint would also be true of no rings at all. */
  it("still covers the road itself, with no seam left down the middle", () => {
    show({ measure: "temperature", coordinates: HAIRPIN, points: WARMING });
    const [longitude, latitude] = HAIRPIN[10] as Position;
    const painting = washFeatures().filter((feature) => covers(feature, longitude, latitude));

    expect(painting).toHaveLength(1);
  });

  // A closed ring gives polygon-clipping self-intersections a hairpin never
  // does, which makes this the slowest test in the file by a wide margin.
  it("leaves the middle of a loop alone, rather than filling it in", () => {
    show({ measure: "temperature", coordinates: LOOP, points: WARMING });
    const features = washFeatures();

    expect(features.filter((feature) => covers(feature, 8, 49))).toEqual([]);
    expect(overlapping(features)).toEqual([]);
  });

  /*
   * The corridor's width comes from the forecast's own grid cell, so a forecast
   * from a coarser model has to be drawn visibly broader — the drawing must not
   * claim more precision than the model has.
   */
  it("widens when the lead time puts the forecast on a coarser grid", () => {
    show({ measure: "temperature", points: WARMING, samples: samplesLeading(1) });
    const fine = latitudeSpan(washFeatures());
    drawn.layers = [];
    drawn.sources = [];
    show({ measure: "temperature", points: WARMING, samples: samplesLeading(100) });
    const coarse = latitudeSpan(washFeatures());

    expect(coarse).toBeGreaterThan(fine);
  });

  it("fades outward in rings, the outermost the faintest", () => {
    show({ measure: "temperature", points: WARMING });
    const alphas = [...new Set(washFeatures().map((feature) => feature.properties.opacity))].sort(
      (a, b) => b - a,
    );

    // One core at full strength and a fade stepping down toward nothing, so the
    // corridor has no edge of its own for the eye to read as a front.
    expect(alphas[0]).toBeCloseTo(WASH_OPACITY, 10);
    expect(alphas).toHaveLength(FADE_RINGS + 1);
    expect(alphas[alphas.length - 1] ?? 0).toBeGreaterThan(0);
  });
});

describe("the bands the wash is painted in", () => {
  it("cuts a run where the reading crosses into the next band", () => {
    const readings: ScalarSample[] = [
      { distanceMetres: 0, value: 0 },
      { distanceMetres: TOTAL_METRES / 2, value: 3 },
      { distanceMetres: TOTAL_METRES, value: 8 },
    ];
    const runs = bandRuns(readings, TOTAL_METRES, RAIN);

    // Nothing, light, moderate, heavy: rain rises through all four, each run
    // starting where the last one ended.
    expect(runs.map((run) => run.band)).toEqual([0, 1, 2, 3]);
    expect(runs[0]?.fromMetres).toBe(0);
    expect(runs[runs.length - 1]?.toMetres).toBe(TOTAL_METRES);
    for (const [index, run] of runs.entries()) {
      expect(run.fromMetres).toBe(runs[index - 1]?.toMetres ?? 0);
    }
  });

  it("cuts it where the reading actually crosses, not at the next sample", () => {
    const readings: ScalarSample[] = [
      { distanceMetres: 0, value: 0 },
      { distanceMetres: TOTAL_METRES / 2, value: 3 },
      { distanceMetres: TOTAL_METRES, value: 8 },
    ];
    const [, light] = bandRuns(readings, TOTAL_METRES, RAIN);

    // 0 mm at the start rising to 3 mm half way along: 0.2 mm — the cut
    // between dry and light rain — falls a fifteenth of the way into that.
    const crossing = (0.2 / 3) * (TOTAL_METRES / 2);
    expect(light?.fromMetres ?? 0).toBeGreaterThanOrEqual(crossing);
    expect((light?.fromMetres ?? 0) - crossing).toBeLessThan(BAND_SCAN_METRES);
  });

  it("builds nothing at all for a band that paints nothing", () => {
    show({ points: RISING_RAIN });
    const dry = RAIN.colour(0, false);

    expect(washFeatures().length).toBeGreaterThan(0);
    for (const feature of washFeatures()) {
      expect(feature.properties.colour).not.toBe(dry);
      expect(feature.properties.opacity).toBeGreaterThan(0);
    }
  });

  it("draws no corridor at all for a ride that is dry throughout", () => {
    show({
      points: [0, 0, 0].map((millimetres) => point({ precipitationMillimetres: millimetres })),
    });

    expect(washFeatures()).toHaveLength(0);
    // The readings are still there to be read, which is the whole of what the
    // wash says to someone not looking at the canvas.
    expect(screen.getByRole("table", { name: /Rain along the route/ })).toBeInTheDocument();
  });

  it("takes the ramp of the cartography actually loaded", () => {
    show({
      measure: "temperature",
      points: [12, 12, 12].map((celsius) => point({ apparentTemperatureCelsius: celsius })),
      dark: true,
    });

    expect(washFeatures()[0]?.properties.colour).toBe(
      TEMPERATURE.colour(TEMPERATURE.band(12), true),
    );
  });
});

describe("what the wash says without the map", () => {
  it("gives every reading in words", () => {
    show({ points: RISING_RAIN });

    for (const millimetres of [0, 3, 8]) {
      expect(screen.getByText(RAIN.words(millimetres))).toBeInTheDocument();
    }
  });

  it("names the measure the readings belong to", () => {
    show({ points: RISING_RAIN });

    expect(screen.getByRole("table", { name: /Rain along the route/ })).toBeInTheDocument();
  });
});

describe("when there is nothing to wash", () => {
  it("draws nothing until a measure is asked for", () => {
    const { container } = show({ measure: null, points: RISING_RAIN });

    expect(washLayer()).toBeUndefined();
    expect(container).toBeEmptyDOMElement();
  });

  it("draws nothing without samples — no start time, or no predicted moving time", () => {
    const { container } = show({ samples: [], points: [] });

    expect(washLayer()).toBeUndefined();
    expect(container).toBeEmptyDOMElement();
  });

  it("draws nothing while the forecast is still on its way", () => {
    const { container } = show({ points: RISING_RAIN, seed: false });

    expect(washLayer()).toBeUndefined();
    expect(container).toBeEmptyDOMElement();
  });
});
