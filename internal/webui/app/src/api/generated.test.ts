import { afterEach, describe, expect, it, vi } from "vitest";
import { getRouteGeometry, getSyncRuns, getWeather, runTask } from "./generated";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("generated browser operations", () => {
  it("serializes each weather point and keeps GeoJSON acceptable", async () => {
    const fetchMock = vi.fn<(input: RequestInfo | URL, init?: RequestInit) => Promise<Response>>(
      async () => new Response(JSON.stringify({ points: [] })),
    );
    vi.stubGlobal("fetch", fetchMock);

    await getWeather({ point: ["49,8,2026-08-25T12:00:00Z", "50,9,2026-08-25T13:00:00Z"] });
    await getRouteGeometry("veloplanner", 12, 1);

    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "/v1/weather?point=49%2C8%2C2026-08-25T12%3A00%3A00Z&point=50%2C9%2C2026-08-25T13%3A00%3A00Z",
    );
    expect(new Headers(fetchMock.mock.calls[1]?.[1]?.headers).get("Accept")).toContain(
      "application/geo+json",
    );
  });

  it("returns the documented accepted envelope", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(JSON.stringify({ status: "accepted" }), { status: 202 })),
    );

    await expect(runTask("sync")).resolves.toMatchObject({
      status: 202,
      data: { status: "accepted" },
    });
  });

  it("surfaces documented failures as ApiError", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ error: { code: "task_in_progress", message: "busy" } }), {
            status: 409,
          }),
      ),
    );

    await expect(runTask("sync")).rejects.toMatchObject({
      name: "ApiError",
      status: 409,
      code: "task_in_progress",
    });
  });

  it("uses the cursor generated from after", async () => {
    const fetchMock = vi.fn<(input: RequestInfo | URL, init?: RequestInit) => Promise<Response>>(
      async () => new Response(JSON.stringify({ runs: [] })),
    );
    vi.stubGlobal("fetch", fetchMock);

    await getSyncRuns({ limit: 10, after: "older" });

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/v1/sync/runs?limit=10&after=older");
  });
});
