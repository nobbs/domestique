import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, domestiqueRequest } from "./request";

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
