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
import { formatClock } from "../../lib/format";
import { PADDING } from "../../lib/plotAxis";
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
  fill?: boolean;
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
        fill={options.fill ?? false}
      />
    </QueryClientProvider>,
  );
}

/**
 * A strip wide enough for its tiles to still carry a wind glyph.
 *
 * jsdom reports every element as nought wide, and a tile that narrow drops its
 * wind before anything else — so without this the arrow under test is never
 * drawn at all and the assertion passes against an empty strip.
 */
function widen(pixels: number): () => void {
  const original = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "clientWidth");
  Object.defineProperty(HTMLElement.prototype, "clientWidth", {
    configurable: true,
    get: () => pixels,
  });

  return () => {
    if (original) {
      Object.defineProperty(HTMLElement.prototype, "clientWidth", original);

      return;
    }
    // jsdom carries `clientWidth` on `Element.prototype`, so there is no own
    // property here to put back and restoring one would be a no-op — deleting
    // the override is what uncovers the inherited accessor again. Left in
    // place it would report 900 to every later test in this worker.
    Reflect.deleteProperty(HTMLElement.prototype, "clientWidth");
  };
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

  it("says where and when each reading is reached", () => {
    const restore = widen(1200);
    try {
      const coordinates = road();
      const samples = forecastSamples(coordinates, movingTime(coordinates), START_AT);
      const { container } = renderStrip({
        coordinates,
        samples,
        seed: forecastFor(samples.length),
      });
      const first = samples[0];

      expect(first).toBeDefined();
      expect(container).toHaveTextContent(`0.0 km · ${formatClock(first?.arrivalAt ?? START_AT)}`);
    } finally {
      restore();
    }
  });

  it("keeps only the clock on a tile too narrow for the distance, and neither when narrower", () => {
    // A long enough ride for the axis's own minimum width to still leave
    // every tile narrow.
    const coordinates = road(160);
    const samples = forecastSamples(coordinates, movingTime(coordinates), START_AT);
    const seed = forecastFor(samples.length);
    const second = samples[1];
    expect(second).toBeDefined();
    const clock = formatClock(second?.arrivalAt ?? START_AT);
    expect(samples.length).toBeGreaterThanOrEqual(10);

    let restore = widen(50 * samples.length + PADDING.left + PADDING.right);
    try {
      const { container } = renderStrip({ coordinates, samples, seed });
      expect(container).toHaveTextContent(clock);
      expect(container).not.toHaveTextContent("km ·");
    } finally {
      restore();
    }

    restore = widen(20 * samples.length + PADDING.left + PADDING.right);
    try {
      const { container } = renderStrip({ coordinates, samples, seed });
      expect(container).not.toHaveTextContent(clock);
    } finally {
      restore();
    }
  });

  it("grows the tile row to its flex parent's height instead of a fixed one, when asked", () => {
    const coordinates = road();
    const samples = forecastSamples(coordinates, movingTime(coordinates), START_AT);
    const seed = forecastFor(samples.length);
    const fixed = renderStrip({ coordinates, samples, seed });
    const fixedBox = fixed.container.querySelector(".border") as HTMLElement;
    expect(fixedBox.style.height).toBe("76px");
    // The tiles themselves, not just the box around them — a fixed height left
    // on a tile after this box grows would strand it at the box's own top.
    for (const tile of fixed.container.querySelectorAll<HTMLElement>(".absolute.top-0")) {
      expect(tile.style.height).toBe("76px");
      expect(tile.style.bottom).toBe("");
    }
    fixed.unmount();

    const grown = renderStrip({ coordinates, samples, seed, fill: true });
    const grownBox = grown.container.querySelector(".border") as HTMLElement;
    expect(grownBox.style.height).toBe("");
    expect(grownBox).toHaveClass("h-full");
    for (const tile of grown.container.querySelectorAll<HTMLElement>(".absolute.top-0")) {
      expect(tile.style.height).toBe("");
      expect(tile.style.bottom).toBe("0px");
    }
  });

  /*
   * The frame, not the pixels. A tile's arrow and the arrows on the map are the
   * same fact, and they once disagreed by the road's own bearing — the tile
   * measured the wind against the rider, the map against north. On this
   * due-east road that gap is the whole reading: rider-relative, this same wind
   * draws at 0°.
   */
  it("points the tile's arrow the way the air is going, in the compass frame", () => {
    const restore = widen(900);
    try {
      const coordinates = road();
      const samples = forecastSamples(coordinates, movingTime(coordinates), START_AT);
      const { container } = renderStrip({
        coordinates,
        samples,
        seed: forecastFor(samples.length),
      });
      const arrows = [...container.querySelectorAll("svg")].filter((svg) =>
        svg.getAttribute("style")?.includes("rotate"),
      );

      expect(arrows.length).toBeGreaterThan(0);
      // The fixture's wind is from the west, so the air is going due east.
      for (const arrow of arrows) {
        expect(arrow).toHaveStyle({ transform: "rotate(90deg)" });
      }
    } finally {
      restore();
    }
  });

  it("leaves the width it stubbed behind it, rather than in the next test", () => {
    const restore = widen(900);
    expect(document.createElement("div").clientWidth).toBe(900);

    restore();

    expect(document.createElement("div").clientWidth).toBe(0);
  });

  it("names that direction for a reader who cannot see the glyph", () => {
    const restore = widen(900);
    try {
      const coordinates = road();
      const samples = forecastSamples(coordinates, movingTime(coordinates), START_AT);
      renderStrip({ coordinates, samples, seed: forecastFor(samples.length) });

      expect(screen.getAllByLabelText(/toward the east/).length).toBeGreaterThan(0);
    } finally {
      restore();
    }
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

  it("reserves the chart's gutters by default, and drops them when inset is false", () => {
    const coordinates = road();
    const samples = forecastSamples(coordinates, movingTime(coordinates), START_AT);
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    client.setQueryData(weatherQuery(samples).queryKey, forecastFor(samples.length));
    const distances = cumulativeMetres(coordinates);

    const { container, rerender } = render(
      <QueryClientProvider client={client}>
        <ForecastStrip
          samples={samples}
          coordinates={coordinates}
          startMetres={0}
          endMetres={distances[distances.length - 1] ?? 0}
        />
      </QueryClientProvider>,
    );
    const gutter = container.querySelector("[role='group'] > div") as HTMLElement;
    expect(gutter.style.paddingLeft).not.toBe("0px");

    rerender(
      <QueryClientProvider client={client}>
        <ForecastStrip
          samples={samples}
          coordinates={coordinates}
          startMetres={0}
          endMetres={distances[distances.length - 1] ?? 0}
          inset={false}
        />
      </QueryClientProvider>,
    );
    const insetOff = container.querySelector("[role='group'] > div") as HTMLElement;
    expect(insetOff.style.paddingLeft).toBe("0px");
  });

  it("omits the resolution sentence when caption is false", () => {
    const coordinates = road();
    const samples = forecastSamples(coordinates, movingTime(coordinates), START_AT);
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    client.setQueryData(weatherQuery(samples).queryKey, forecastFor(samples.length));

    render(
      <QueryClientProvider client={client}>
        <ForecastStrip
          samples={samples}
          coordinates={coordinates}
          startMetres={0}
          endMetres={cumulativeMetres(coordinates).at(-1) ?? 0}
          caption={false}
        />
      </QueryClientProvider>,
    );

    expect(screen.queryByText(/ICON-D2|ICON-EU|coarser global guidance/)).not.toBeInTheDocument();
  });
});
