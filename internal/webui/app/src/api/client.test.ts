import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, fetchStageGeometry, fetchStages } from "./client";
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
});
