import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, domestiqueBinaryRequest, domestiqueRequest, unwrap } from "./request";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("domestiqueRequest", () => {
  it("sends an expired session to sign in again, and still throws", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ error: { code: "unauthorized", message: "no session" } }), {
            status: 401,
          }),
      ),
    );
    const assign = vi.fn();
    vi.stubGlobal("location", { assign });

    await expect(domestiqueRequest("/v1/routes", { method: "GET" })).rejects.toBeInstanceOf(
      ApiError,
    );

    expect(assign).toHaveBeenCalledWith("/auth/login");
  });

  it("does not redirect on a failure that is not a session problem", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () => new Response(JSON.stringify({ error: { code: "not_found" } }), { status: 404 }),
      ),
    );
    const assign = vi.fn();
    vi.stubGlobal("location", { assign });

    await expect(domestiqueRequest("/v1/routes", { method: "GET" })).rejects.toBeInstanceOf(
      ApiError,
    );

    expect(assign).not.toHaveBeenCalled();
  });
});

describe("domestiqueBinaryRequest", () => {
  it("resolves a blob without parsing it as JSON", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("not json", { status: 200 })),
    );

    const result = await domestiqueBinaryRequest<{ data: Blob; status: number }>(
      "/v1/weather-grid/object",
      { method: "GET" },
    );

    expect(result.status).toBe(200);
    expect(await result.data.text()).toBe("not json");
  });

  it("reads the same structured error envelope every other operation gets on failure", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({ error: { code: "provider_unavailable", message: "gone" } }),
            { status: 502 },
          ),
      ),
    );

    const error = await domestiqueBinaryRequest("/v1/weather-grid/object", {
      method: "GET",
    }).catch((caught: unknown) => caught);

    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).code).toBe("provider_unavailable");
    expect((error as ApiError).message).toBe("gone");
  });

  it("sends an expired session to sign in again on failure, same as domestiqueRequest", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(null, { status: 401 })),
    );
    const assign = vi.fn();
    vi.stubGlobal("location", { assign });

    await expect(
      domestiqueBinaryRequest("/v1/weather-grid/object", { method: "GET" }),
    ).rejects.toBeInstanceOf(ApiError);

    expect(assign).toHaveBeenCalledWith("/auth/login");
  });
});

describe("unwrap", () => {
  it("reads the body out of domestiqueRequest's own envelope", () => {
    expect(
      unwrap({
        data: { reference_time: "2026-09-05T12:00:00Z" },
        status: 200,
        headers: new Headers(),
      }),
    ).toEqual({ reference_time: "2026-09-05T12:00:00Z" });
  });

  it("passes a value straight through when it is not an envelope", () => {
    expect(unwrap({ reference_time: "2026-09-05T12:00:00Z" })).toEqual({
      reference_time: "2026-09-05T12:00:00Z",
    });
  });

  it("does not unwrap a legitimate payload that merely has its own data field", () => {
    const payload = { data: [1, 2, 3] };
    expect(unwrap<typeof payload>(payload)).toBe(payload);
  });

  it("does not unwrap a payload whose status/headers fields are the wrong runtime type", () => {
    const payload = { data: [1, 2, 3], status: "200", headers: {} };
    expect(unwrap<typeof payload>(payload)).toBe(payload);
  });
});
