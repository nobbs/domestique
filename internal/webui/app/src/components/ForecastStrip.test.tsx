/**
 * The strip, without a real Open-Meteo behind it.
 *
 * The weather query is seeded into the cache the way `RoutesPage.test.tsx`
 * seeds the route library — `weatherQuery`'s own key, so the component's own
 * `useQuery` call reads exactly what this test put there — with a catch-all
 * 404 `fetch` stub underneath so an unseeded request is loud rather than a
 * silent hang.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { weatherQuery } from "../api/queries";
import type { Position, WeatherForecast } from "../api/types";
import { forecastSamples } from "../lib/forecastSamples";
import { cumulativeMetres } from "../lib/profile";
import { ForecastStrip } from "./ForecastStrip";

/** A due-east road, so `wind.ts` has an unambiguous bearing to project against. */
function road(pointCount = 12): Position[] {
  return Array.from({ length: pointCount }, (_, index): Position => [8 + index * 0.01, 49]);
}

const START_AT = new Date("2026-08-25T07:00:00Z");

/** Predicted moving time at each point, two minutes apart — enough for a few samples. */
function movingTime(coordinates: Position[]): number[] {
  return coordinates.map((_, index) => index * 120);
}

function forecastFor(sampleCount: number): WeatherForecast {
  return {
    points: Array.from({ length: sampleCount }, (_, index) => ({
      time: new Date(START_AT.getTime() + index * 60_000).toISOString(),
      temperatureCelsius: 12,
      apparentTemperatureCelsius: 11,
      // Alternating so the precipitation-carrying cells and the dry ones are
      // both exercised.
      precipitationMillimetres: index % 2 === 0 ? 0 : 2.4,
      precipitationProbabilityPercent: index % 2 === 0 ? 0 : 55,
      windSpeedKmh: 18,
      // From the west: a due-east road reads this as a tailwind.
      windDirectionDegrees: 270,
      weatherCode: index % 2 === 0 ? 1 : 61,
    })),
  };
}

function renderStrip(options: {
  samples?: ReturnType<typeof forecastSamples>;
  coordinates?: Position[];
  seed?: WeatherForecast;
  unitSystem?: "metric" | "imperial";
}) {
  const coordinates = options.coordinates ?? road();
  const samples =
    options.samples ?? forecastSamples(coordinates, movingTime(coordinates), START_AT);
  const distances = cumulativeMetres(coordinates);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  if (options.seed) {
    client.setQueryData(weatherQuery(samples).queryKey, options.seed);
  }

  return render(
    <QueryClientProvider client={client}>
      <ForecastStrip
        samples={samples}
        coordinates={coordinates}
        startMetres={0}
        endMetres={distances[distances.length - 1] ?? 0}
        unitSystem={options.unitSystem ?? "metric"}
      />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("{}", { status: 404 })),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("ForecastStrip", () => {
  it("draws one cell per sample once a forecast has arrived", () => {
    const coordinates = road();
    const samples = forecastSamples(coordinates, movingTime(coordinates), START_AT);
    renderStrip({ coordinates, samples, seed: forecastFor(samples.length) });

    expect(screen.getByRole("img")).toBeInTheDocument();
    // One row of readings per sample, in the screen-reader table.
    expect(screen.getAllByRole("row")).toHaveLength(samples.length + 1); // + the header row
  });

  it("renders nothing without any samples — no start time chosen", () => {
    const { container } = renderStrip({ samples: [], seed: forecastFor(0) });

    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing for a stage nothing has predicted a moving time for", () => {
    const coordinates = road();
    // No predicted duration at all: forecastSamples refuses to guess one.
    const samples = forecastSamples(coordinates, undefined, START_AT);
    const { container } = renderStrip({ coordinates, samples });

    expect(samples).toEqual([]);
    expect(container).toBeEmptyDOMElement();
  });

  it("says the forecast is unavailable on a 502, rather than breaking", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              error: {
                code: "provider_unavailable",
                message: "the weather provider could not be reached",
              },
            }),
            { status: 502 },
          ),
      ),
    );
    renderStrip({});

    expect(await screen.findByText(/forecast is unavailable/i)).toBeInTheDocument();
  });

  it("honours the unit preference in every figure the strip reports", () => {
    const coordinates = road();
    const samples = forecastSamples(coordinates, movingTime(coordinates), START_AT);
    renderStrip({
      coordinates,
      samples,
      seed: forecastFor(samples.length),
      unitSystem: "imperial",
    });

    const table = screen.getByRole("table");
    expect(table).toHaveTextContent("°F");
    expect(table).toHaveTextContent("mph");
  });

  it("reads a tailwind for a due-east road in a westerly wind", () => {
    const coordinates = road();
    const samples = forecastSamples(coordinates, movingTime(coordinates), START_AT);
    renderStrip({ coordinates, samples, seed: forecastFor(samples.length) });

    expect(screen.getByRole("table")).toHaveTextContent(/Tailwind/i);
  });
});
