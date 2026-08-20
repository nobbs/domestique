import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ApiError,
  fetchStageGeometry,
  fetchStages,
  fetchSyncRuns,
  setSyncSchedule,
  triggerSync,
} from "./client";
import { ContractError } from "./parse";

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

    await fetchStages();

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

  it("surfaces the service's safe error envelope", async () => {
    respondWith(404, { error: { code: "not_found", message: "resource was not found" } });

    const failure = await fetchStageGeometry(12, 1).catch((error: unknown) => error);

    expect(failure).toBeInstanceOf(ApiError);
    const apiError = failure as ApiError;
    expect(apiError.status).toBe(404);
    expect(apiError.code).toBe("not_found");
    expect(apiError.isNotFound).toBe(true);
  });

  it("reports an unauthenticated caller rather than parsing the body", async () => {
    respondWith(401, { error: { code: "unauthorized", message: "tailnet identity is required" } });

    const failure = await fetchStages().catch((error: unknown) => error);

    expect(failure).toBeInstanceOf(ApiError);
    expect((failure as ApiError).status).toBe(401);
  });

  it("still fails cleanly when an error body is not JSON", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("gateway exploded", { status: 502 })),
    );

    const failure = await fetchStages().catch((error: unknown) => error);

    expect(failure).toBeInstanceOf(ApiError);
    expect((failure as ApiError).status).toBe(502);
  });

  it("rejects a success payload that does not match the contract", async () => {
    respondWith(200, { stages: [{ route_id: "not-a-number" }] });

    await expect(fetchStages()).rejects.toBeInstanceOf(ContractError);
  });

  it("names the request a contract failure came back from", async () => {
    respondWith(200, { stages: [{ route_id: "not-a-number" }] });

    const failure = (await fetchStages().catch((error: unknown) => error)) as ContractError;

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
});
