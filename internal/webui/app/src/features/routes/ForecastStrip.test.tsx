/**
 * The strip, without a real Open-Meteo behind it.
 *
 * The weather query is seeded into the cache the way `AtlasPage.test.tsx` seeds
 * the route library — `weatherQuery`'s own key, so the component's own
 * `useQuery` call reads exactly what this test put there — with a catch-all 404
 * `fetch` stub underneath so an unseeded request is loud rather than a silent
 * hang.
 *
 * What is tested here is the part that is not drawing: which of the several
 * ways there can be no forecast leads to silence and which to a sentence, and
 * whether a failure is reported as the provider's or as this page's. A tile's
 * pixels are Chromatic's business; a wrong error message sends a reader to
 * check whether Open-Meteo is down over arithmetic done here.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { weatherQuery } from "../../api/queries";
import type { Position, WeatherForecast } from "../../api/types";
import { forecastSamples } from "../../lib/forecastSamples";
import { cumulativeMetres } from "../../lib/profile";
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
      precipitationMillimetres: index % 2 === 0 ? 0 : 2.4,
      precipitationProbabilityPercent: index % 2 === 0 ? 0 : 55,
      windSpeedKmh: 18,
      // From the west: a due-east road reads this as a tailwind.
      windDirectionDegrees: 270,
      weatherCode: index % 2 === 0 ? 1 : 61,
      cloudCoverPercent: index % 2 === 0 ? 10 : 90,
    })),
  };
}

function renderStrip(options: {
  samples?: ReturnType<typeof forecastSamples>;
  coordinates?: Position[];
  seed?: WeatherForecast;
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
        unitSystem="metric"
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
  it("draws one tile per reading once a forecast has arrived", () => {
    const coordinates = road();
    const samples = forecastSamples(coordinates, movingTime(coordinates), START_AT);
    const { container } = renderStrip({ coordinates, samples, seed: forecastFor(samples.length) });

    expect(samples.length).toBeGreaterThan(0);
    // A tile carries its reading's temperature, which is the figure that
    // survives longest as tiles narrow.
    expect(container.querySelectorAll("[style*='color-mix']").length).toBeGreaterThanOrEqual(
      samples.length,
    );
  });

  it("says how sharp a forecast this far ahead can be", () => {
    const coordinates = road();
    const samples = forecastSamples(coordinates, movingTime(coordinates), START_AT);
    renderStrip({ coordinates, samples, seed: forecastFor(samples.length) });

    expect(screen.getByText(/ICON-D2|ICON-EU|coarser global guidance/)).toBeInTheDocument();
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

    expect(container).toBeEmptyDOMElement();
  });

  it("says the forecast is unavailable on a 502, rather than breaking", async () => {
    // The request is allowed to fail for real rather than the cache being told
    // it already did: the branch turns on the error the client threw, and a
    // hand-built one is a guess at that.
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              error: { code: "provider_unavailable", message: "provider unreachable" },
            }),
            { status: 502 },
          ),
      ),
    );
    renderStrip({});

    expect(await screen.findByText(/forecast is unavailable/i)).toBeInTheDocument();
  });

  it("does not blame the provider for a request the endpoint refused", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              error: { code: "invalid_request", message: "too many points" },
            }),
            { status: 400 },
          ),
      ),
    );
    renderStrip({});

    expect(await screen.findByText(/could not be requested/i)).toBeInTheDocument();
  });
});
