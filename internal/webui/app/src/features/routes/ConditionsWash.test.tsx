/**
 * The forecast wash, asked about without a canvas.
 *
 * The same approach `RouteOverlay.test.tsx` takes: MapLibre's bindings are
 * fakes that record what they were handed, so a question about the ink can be
 * asked of a map that never draws anything. The forecast itself is seeded into
 * the query cache under `weatherQuery`'s own key, as `ForecastStrip.test.tsx`
 * seeds it, with a catch-all 404 `fetch` underneath so an unseeded request is
 * loud rather than a silent hang.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { weatherQuery } from "../../api/queries";
import type { Position, WeatherForecast, WeatherPoint } from "../../api/types";
import { CartographyProvider } from "../../components/map/CartographyContext";
import type { ForecastSample } from "../../lib/forecastSamples";
import { MEASURES, type MeasureKey } from "../../lib/measures";
import { cumulativeMetres } from "../../lib/profile";

interface LayerRecord {
  id: string;
  beforeId?: string;
  paint: Record<string, unknown>;
}

interface SourceRecord {
  id: string;
  lineMetrics?: boolean | undefined;
  data: unknown;
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
    drawn.sources.push({ id: props.id, lineMetrics: props.lineMetrics, data: props.data });

    return <>{props.children}</>;
  },
}));

const { BAND_SCAN_METRES, ConditionsWash, WASH_LAYER_ID } = await import("./ConditionsWash");

/** A due-east road of about 29 km, long enough for a band to change along it. */
const ROAD: Position[] = Array.from({ length: 41 }, (_, index): Position => [8 + index * 0.01, 49]);
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

/** Three samples spread evenly along the road, the first `leadHours` from now. */
function samplesLeading(leadHours: number): ForecastSample[] {
  return [0, 0.5, 1].map((share) => ({
    position: ROAD[Math.round(share * (ROAD.length - 1))] as Position,
    distanceMetres: share * TOTAL_METRES,
    arrivalAt: new Date(Date.now() + (leadHours + share) * 3_600_000),
  }));
}

function show(options: {
  measure?: MeasureKey | null;
  samples?: ForecastSample[];
  points?: WeatherPoint[];
  seed?: boolean;
  dark?: boolean;
}) {
  const samples = options.samples ?? samplesLeading(1);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  if (options.seed !== false && options.points) {
    const forecast: WeatherForecast = { points: options.points };
    client.setQueryData(weatherQuery(samples).queryKey, forecast);
  }

  return render(
    <QueryClientProvider client={client}>
      <CartographyProvider dark={options.dark ?? false}>
        <ConditionsWash
          coordinates={ROAD}
          samples={samples}
          measure={options.measure === undefined ? "rain" : options.measure}
          beforeId="route-window-halo"
          unitSystem="metric"
        />
      </CartographyProvider>
    </QueryClientProvider>,
  );
}

function washLayer(): LayerRecord | undefined {
  return drawn.layers.find((layer) => layer.id === WASH_LAYER_ID);
}

/** The `step` expression's stops as `[progress, colour]` pairs, the first at 0. */
function gradientStops(): Array<[number, string]> {
  const gradient = washLayer()?.paint["line-gradient"] as unknown[] | undefined;
  if (!gradient) {
    return [];
  }
  const stops: Array<[number, string]> = [[0, gradient[2] as string]];
  for (let index = 3; index < gradient.length; index += 2) {
    stops.push([gradient[index] as number, gradient[index + 1] as string]);
  }

  return stops;
}

const RAIN = MEASURES.find((measure) => measure.key === "rain") as (typeof MEASURES)[number];
const TEMPERATURE = MEASURES.find(
  (measure) => measure.key === "temperature",
) as (typeof MEASURES)[number];

/** Rain rising from nothing to a downpour: every band in order along one road. */
const RISING_RAIN = [0, 3, 8].map((millimetres) =>
  point({ precipitationMillimetres: millimetres }),
);

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

describe("the corridor the wash is drawn in", () => {
  it("carries line metrics, without which a gradient has no progress to run over", () => {
    show({ points: RISING_RAIN });

    expect(drawn.sources[0]?.lineMetrics).toBe(true);
  });

  it("draws one LineString, so the gradient runs once end to end", () => {
    show({ points: RISING_RAIN });

    expect(drawn.sources[0]?.data).toMatchObject({ geometry: { type: "LineString" } });
  });

  it("sits under everything the route itself draws", () => {
    show({ points: RISING_RAIN });

    expect(washLayer()?.beforeId).toBe("route-window-halo");
  });

  /*
   * The corridor's width comes from the forecast's own grid cell, so a forecast
   * from a coarser model has to be drawn visibly broader — the drawing must not
   * claim more precision than the model has.
   */
  it("widens when the lead time puts the forecast on a coarser grid", () => {
    show({ points: RISING_RAIN, samples: samplesLeading(1) });
    const fine = washLayer()?.paint["line-width"] as unknown[];
    drawn.layers = [];
    show({ points: RISING_RAIN, samples: samplesLeading(100) });
    const coarse = washLayer()?.paint["line-width"] as unknown[];

    // Index 4 is the width at zoom 0; the whole ramp scales with it.
    expect(coarse[4] as number).toBeGreaterThan(fine[4] as number);
  });

  it("fades over the ground between the core radius and the edge, so it has no boundary", () => {
    show({ points: RISING_RAIN });
    const width = washLayer()?.paint["line-width"] as unknown[];
    const blur = washLayer()?.paint["line-blur"] as unknown[];

    // 1.25 cells of fade inside a 4-cell width: full strength to the core
    // radius, gone at the edge.
    expect(blur[4] as number).toBeCloseTo(((width[4] as number) * 1.25) / 4, 6);
  });
});

describe("the bands the wash is painted in", () => {
  it("puts a stop where the reading crosses into the next band", () => {
    show({ points: RISING_RAIN });
    const stops = gradientStops();

    // Nothing, light, moderate, heavy: rain rises through all four.
    expect(stops.map(([, colour]) => colour)).toHaveLength(RAIN.bands.length);
    expect(stops.map(([progress]) => progress)).toEqual(
      [...stops.map(([progress]) => progress)].sort((a, b) => a - b),
    );
  });

  it("puts that stop where the reading actually crosses, not at the next sample", () => {
    show({ points: RISING_RAIN });
    const [, light] = gradientStops();

    // 0 mm at the start rising to 3 mm half way along: 0.2 mm — the cut
    // between dry and light rain — falls a fifteenth of the way into that.
    const crossing = (0.2 / 3) * (TOTAL_METRES / 2);
    const stopMetres = (light?.[0] ?? 0) * TOTAL_METRES;
    expect(stopMetres).toBeGreaterThanOrEqual(crossing);
    expect(stopMetres - crossing).toBeLessThan(BAND_SCAN_METRES);
  });

  it("emits a transparent lowest band rather than a pale one", () => {
    show({ points: RISING_RAIN });
    const [dry] = gradientStops();

    expect(dry?.[1]).toContain(", 0)");
  });

  it("paints every band of a measure that never says nothing", () => {
    show({
      measure: "temperature",
      points: [-5, 12, 30].map((celsius) => point({ apparentTemperatureCelsius: celsius })),
    });

    for (const [, colour] of gradientStops()) {
      expect(colour).not.toContain(", 0)");
    }
  });

  it("still emits a stop pair for a route that stays in one band", () => {
    show({
      measure: "temperature",
      points: [12, 12, 12].map((celsius) => point({ apparentTemperatureCelsius: celsius })),
    });
    const stops = gradientStops();

    expect(stops).toHaveLength(2);
    expect(stops[0]?.[1]).toBe(stops[1]?.[1]);
  });

  it("takes the ramp of the cartography actually loaded", () => {
    show({
      measure: "temperature",
      points: [12, 12, 12].map((celsius) => point({ apparentTemperatureCelsius: celsius })),
      dark: true,
    });
    const [first] = gradientStops();
    const hex = TEMPERATURE.colour(TEMPERATURE.band(12), true);
    const value = Number.parseInt(hex.slice(1), 16);

    expect(first?.[1]).toBe(
      `rgba(${(value >> 16) & 255}, ${(value >> 8) & 255}, ${value & 255}, 1)`,
    );
  });
});

describe("what the wash says without the map", () => {
  it("gives every reading in words, in the reader's own units", () => {
    show({ points: RISING_RAIN });

    for (const millimetres of [0, 3, 8]) {
      expect(screen.getByText(RAIN.words(millimetres, "metric"))).toBeInTheDocument();
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
