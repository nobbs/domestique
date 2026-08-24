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
  startAt?: Date;
  startMetres?: number;
  endMetres?: number;
}) {
  const coordinates = options.coordinates ?? road();
  const samples =
    options.samples ??
    forecastSamples(coordinates, movingTime(coordinates), options.startAt ?? START_AT);
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
        startMetres={options.startMetres ?? 0}
        endMetres={options.endMetres ?? distances[distances.length - 1] ?? 0}
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

  /*
   * A 400 is this page having asked for something the endpoint refuses, not
   * the provider being down. Reporting it as an outage would send a reader off
   * to check whether Open-Meteo is up over arithmetic done here.
   */
  it("does not blame the provider for a request the endpoint refused", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              error: {
                code: "invalid_request",
                message: "the time window is outside what the provider can forecast",
              },
            }),
            { status: 400 },
          ),
      ),
    );
    renderStrip({});

    expect(await screen.findByText(/could not be requested/i)).toBeInTheDocument();
    expect(screen.queryByText(/forecast is unavailable/i)).not.toBeInTheDocument();
  });

  /*
   * The projected component is the number a cyclist wants: a wind leaning
   * across the road is a couple of km/h against you, not its full speed. The
   * raw reading still travels, because that is what any other forecast quotes.
   */
  /*
   * A crosswind still leans one way or the other, and the magnitude alone
   * cannot say which: without the sign, a cross pushing the rider along and
   * one pushing back announce the same number.
   */
  it("says which way a crosswind leans", () => {
    const coordinates = road();
    const samples = forecastSamples(coordinates, movingTime(coordinates), START_AT);
    const leaning = (windDirectionDegrees: number) => {
      const seed = forecastFor(samples.length);
      seed.points = seed.points.map((point) => ({ ...point, windDirectionDegrees }));
      const view = renderStrip({ coordinates, samples, seed });
      const text = screen.getByRole("table").textContent ?? "";
      view.unmount();

      return text;
    };

    // A due-east road: a wind from just north of due south leans forwards,
    // and one from just north of due north leans back.
    expect(leaning(190)).toMatch(/Crosswind, .* with you along the route/);
    expect(leaning(10)).toMatch(/Crosswind, .* against you along the route/);
  });

  it("reports the wind along the route, keeping the raw reading beside it", () => {
    const coordinates = road();
    const samples = forecastSamples(coordinates, movingTime(coordinates), START_AT);
    renderStrip({ coordinates, samples, seed: forecastFor(samples.length) });

    const table = screen.getByRole("table");
    expect(table).toHaveTextContent(/Tailwind .* along the route/i);
    expect(table).toHaveTextContent(/18 km\/h from W \(270°\)/);
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

  /*
   * The felt temperature is the one that decides what goes in the jersey
   * pocket, and the dry-bulb figure alone would be the wrong number to lead
   * with. Both travel; neither replaces the other.
   */
  it("leads with the felt temperature and keeps the actual one beside it", () => {
    const coordinates = road();
    const samples = forecastSamples(coordinates, movingTime(coordinates), START_AT);
    const seed = forecastFor(samples.length);
    seed.points = seed.points.map((point) => ({
      ...point,
      temperatureCelsius: 12,
      apparentTemperatureCelsius: 6,
    }));
    renderStrip({ coordinates, samples, seed });

    const table = screen.getByRole("table");
    // Near freezing keeps a decimal; a dozen degrees does not — see format.ts.
    expect(table).toHaveTextContent(/feels 6\.0\s*°C/);
    expect(table).toHaveTextContent(/12\s*°C actual/);
  });

  /*
   * A dry cell is drawn by having no shading at all, so without words the only
   * signal that no rain was forecast is the absence of colour — the state this
   * page does not allow. "Overcast" alone does not say it either.
   */
  it("says a dry hour is dry rather than leaving it to the missing shading", () => {
    const coordinates = road();
    const samples = forecastSamples(coordinates, movingTime(coordinates), START_AT);
    const seed = forecastFor(samples.length);
    seed.points = seed.points.map((point) => ({
      ...point,
      weatherCode: 3,
      precipitationMillimetres: 0,
      precipitationProbabilityPercent: 0,
    }));
    renderStrip({ coordinates, samples, seed });

    expect(screen.getByRole("table")).toHaveTextContent(/no rain expected/i);
  });

  /*
   * The endpoint resolves every point to its nearest hour, so a point's own
   * `time` is the forecast's clock and not the rider's. Reporting it as the
   * arrival would tell somebody passing at 08:30 that they get there at 09:00.
   */
  it("reports when the rider arrives, not the hour the forecast was resolved to", () => {
    const coordinates = road();
    const samples = forecastSamples(coordinates, movingTime(coordinates), START_AT);
    const seed = forecastFor(samples.length);
    // Every reading resolved to one hour, well away from any arrival.
    seed.points = seed.points.map((point) => ({
      ...point,
      time: new Date("2026-08-25T23:00:00Z").toISOString(),
    }));
    renderStrip({ coordinates, samples, seed });

    const rows = screen.getAllByRole("row").slice(1);
    const arrivals = (samples[0] as { arrivalAt: Date }).arrivalAt.toLocaleString(undefined, {
      dateStyle: "medium",
      timeStyle: "short",
    });
    expect(rows[0]).toHaveTextContent(arrivals);
    expect(screen.getByRole("table")).not.toHaveTextContent("23:00");
  });

  /*
   * The strip is handed whatever stretch the elevation chart is drawing, so
   * dragging out a climb narrows this axis too. A cell for ground outside that
   * stretch has nowhere to sit and must be dropped rather than clamped onto
   * the edge, where it would claim a reading for ground the chart is not
   * showing.
   */
  it("drops the cells outside the stretch the profile is showing", () => {
    const coordinates = road();
    const samples = forecastSamples(coordinates, movingTime(coordinates), START_AT);
    const distances = cumulativeMetres(coordinates);
    const total = distances[distances.length - 1] ?? 0;

    const whole = renderStrip({ coordinates, samples, seed: forecastFor(samples.length) });
    const wholeRows = screen.getAllByRole("row").length;
    whole.unmount();

    renderStrip({
      coordinates,
      samples,
      seed: forecastFor(samples.length),
      startMetres: total / 2,
      endMetres: total,
    });

    expect(screen.getAllByRole("row").length).toBeLessThan(wholeRows);
  });

  /*
   * The endpoint answers 16 days out and the response never names its model,
   * so the only honest signal that a far-off forecast is a coarser one is this
   * line. Every band it can report is a start time a reader can pick.
   */
  it("says how sharp the forecast is, and says it differently further out", () => {
    const coordinates = road();
    const readingFor = (daysAhead: number) => {
      const startAt = new Date(Date.now() + daysAhead * 24 * 60 * 60 * 1000);
      const samples = forecastSamples(coordinates, movingTime(coordinates), startAt);
      const view = renderStrip({ coordinates, samples, seed: forecastFor(samples.length) });
      const text = screen.getByText(/resolution|guidance/i).textContent ?? "";
      view.unmount();

      return text;
    };

    const tomorrow = readingFor(1);
    const inThreeDays = readingFor(3);
    const inTenDays = readingFor(10);

    expect(tomorrow).toMatch(/2 km/);
    expect(inThreeDays).not.toBe(tomorrow);
    expect(inTenDays).not.toBe(inThreeDays);
  });
});
