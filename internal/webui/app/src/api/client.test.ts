import { afterEach, describe, expect, it, vi } from "vitest";
import type { ForecastSample } from "../lib/forecastSamples";
import {
  ApiError,
  fetchRoute,
  fetchRouteGeometry,
  fetchRoutes,
  fetchSyncRuns,
  fetchWeather,
  setSyncSchedule,
  triggerSync,
  triggerTargetSync,
} from "./client";
import { ContractError } from "./parse";

const weatherPointPayload = {
  time: "2026-08-25T09:00:00Z",
  temperature_celsius: 18.2,
  apparent_temperature_celsius: 17.1,
  precipitation_millimetres: 0.4,
  precipitation_probability_percent: 35,
  wind_speed_kmh: 18,
  wind_direction_degrees: 240,
  weather_code: 61,
};

/** [longitude, latitude], deliberately far apart so a transposed pair reads wrong. */
function forecastSample(position: [number, number] = [8.4, 49.0]): ForecastSample {
  return {
    position,
    distanceMetres: 0,
    arrivalAt: new Date("2026-08-25T09:00:00Z"),
  };
}

function respondWith(status: number, body: unknown): void {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify(body), { status })),
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("the API client", () => {
  it("requests the documented path", async () => {
    const fetchMock = vi.fn(
      async () => new Response(JSON.stringify({ stages: [] }), { status: 200 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await fetchRoutes();

    expect(fetchMock).toHaveBeenCalledWith("/v1/routes", expect.anything());
  });

  it("asks for the newest page of history without a cursor, and follows one after", async () => {
    const fetchMock = vi.fn(
      async () => new Response(JSON.stringify({ runs: [] }), { status: 200 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await fetchSyncRuns(undefined, 10);
    await fetchSyncRuns("412", 10);

    expect(fetchMock).toHaveBeenNthCalledWith(1, "/v1/sync/runs?limit=10", expect.anything());
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/v1/sync/runs?limit=10&after=412",
      expect.anything(),
    );
  });

  it("addresses one stage by the provider that issued it", async () => {
    const fetchMock = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            provider: "veloplanner",
            route_id: 12,
            stage: 1,
            title: "Kaiserstuhl Loop",
            route_name: "Kaiserstuhl Loop",
            stage_name: "",
            distance_metres: 1000,
            point_count: 2,
          }),
          { status: 200 },
        ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const stage = await fetchRoute("veloplanner", 12, 1);

    expect(fetchMock).toHaveBeenCalledWith(
      "/v1/providers/veloplanner/routes/12/stages/1",
      expect.anything(),
    );
    expect(stage.provider).toBe("veloplanner");
  });

  it("surfaces the service's safe error envelope", async () => {
    respondWith(404, { error: { code: "not_found", message: "resource was not found" } });

    const failure = await fetchRouteGeometry("veloplanner", 12, 1).catch((error: unknown) => error);

    expect(failure).toBeInstanceOf(ApiError);
    const apiError = failure as ApiError;
    expect(apiError.status).toBe(404);
    expect(apiError.code).toBe("not_found");
    expect(apiError.isNotFound).toBe(true);
  });

  it("reports an unauthenticated caller rather than parsing the body", async () => {
    respondWith(401, { error: { code: "unauthorized", message: "tailnet identity is required" } });

    const failure = await fetchRoutes().catch((error: unknown) => error);

    expect(failure).toBeInstanceOf(ApiError);
    expect((failure as ApiError).status).toBe(401);
  });

  it("still fails cleanly when an error body is not JSON", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("gateway exploded", { status: 502 })),
    );

    const failure = await fetchRoutes().catch((error: unknown) => error);

    expect(failure).toBeInstanceOf(ApiError);
    expect((failure as ApiError).status).toBe(502);
  });

  it("rejects a success payload that does not match the contract", async () => {
    respondWith(200, { stages: [{ route_id: "not-a-number" }] });

    await expect(fetchRoutes()).rejects.toBeInstanceOf(ContractError);
  });

  it("names the request a contract failure came back from", async () => {
    respondWith(200, { stages: [{ provider: "veloplanner", route_id: "not-a-number" }] });

    const failure = (await fetchRoutes().catch((error: unknown) => error)) as ContractError;

    // The endpoint is the half the parsers cannot know, and it is what turns a
    // drift report into something a reader can go and look at.
    expect(failure.endpoint).toBe("GET /v1/routes");
    expect(failure.where).toBe("stages[0].route_id is not a finite number");
    expect(failure.message).toContain("GET /v1/routes");
    expect(failure.message).toContain("stages[0].route_id");
  });

  it("names a mutation's endpoint the same way", async () => {
    respondWith(200, { source: true });

    const failure = (await setSyncSchedule({ source: true, targets: false }).catch(
      (error: unknown) => error,
    )) as ContractError;

    expect(failure.endpoint).toBe("PUT /v1/sync/schedule");
  });

  it("starts the half it was asked for", async () => {
    const fetchMock = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(JSON.stringify({ status: "accepted" })),
    );
    vi.stubGlobal("fetch", fetchMock);

    await triggerSync("source");
    await triggerSync("targets");
    await triggerSync("all");

    expect(fetchMock.mock.calls.map((call) => call[0])).toEqual([
      "/v1/sync/source",
      "/v1/sync/targets",
      "/v1/sync",
    ]);
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({ method: "POST" });
  });

  // A run already in flight is a refusal worth showing, not a silent no-op.
  it("reports a rejected trigger", async () => {
    respondWith(409, {
      error: { code: "sync_in_progress", message: "a synchronization is already running" },
    });

    const failure = await triggerSync("source").catch((error: unknown) => error);

    expect(failure).toBeInstanceOf(ApiError);
    expect((failure as ApiError).code).toBe("sync_in_progress");
  });

  it("triggers exactly the named target", async () => {
    const fetchMock = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(JSON.stringify({ status: "accepted" })),
    );
    vi.stubGlobal("fetch", fetchMock);

    await triggerTargetSync("rider-a");

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/v1/sync/targets/rider-a");
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({ method: "POST" });
  });

  it("reports a rejected target trigger", async () => {
    respondWith(409, {
      error: { code: "sync_in_progress", message: "a synchronization is already running" },
    });

    const failure = await triggerTargetSync("rider-a").catch((error: unknown) => error);

    expect(failure).toBeInstanceOf(ApiError);
    expect((failure as ApiError).code).toBe("sync_in_progress");
  });

  it("sends both schedule switches and reads back what was stored", async () => {
    const fetchMock = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(JSON.stringify({ source: true, targets: false })),
    );
    vi.stubGlobal("fetch", fetchMock);

    const stored = await setSyncSchedule({ source: true, targets: false });

    expect(fetchMock).toHaveBeenCalledWith(
      "/v1/sync/schedule",
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({ source: true, targets: false }),
      }),
    );
    expect(stored).toEqual({ source: true, targets: false });
  });

  // ForecastSample.position is [longitude, latitude] — GeoJSON order — but the
  // wire format is "latitude,longitude,time", the opposite way round. This
  // sample's two coordinates are unmistakably different, so a transposed pair
  // would fail this assertion rather than pass it by coincidence.
  it("builds the weather query with latitude before longitude", async () => {
    const fetchMock = vi.fn(
      async () => new Response(JSON.stringify({ points: [weatherPointPayload] }), { status: 200 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await fetchWeather([forecastSample([8.4, 49.0])]);

    expect(fetchMock).toHaveBeenCalledWith(
      "/v1/weather?point=49%2C8.4%2C2026-08-25T09%3A00%3A00.000Z",
      expect.anything(),
    );
  });

  it("reads a forecast with one point per requested sample", async () => {
    respondWith(200, { points: [weatherPointPayload] });

    const forecast = await fetchWeather([forecastSample()]);

    expect(forecast.points).toHaveLength(1);
    expect(forecast.points[0]).toMatchObject({ temperatureCelsius: 18.2, weatherCode: 61 });
  });

  // The endpoint answers exactly one point per point requested; anything else
  // is drift between this client and the service, not data to render around.
  it("refuses a forecast whose point count disagrees with the request", async () => {
    respondWith(200, { points: [weatherPointPayload, weatherPointPayload] });

    await expect(fetchWeather([forecastSample()])).rejects.toBeInstanceOf(ContractError);
  });
});
