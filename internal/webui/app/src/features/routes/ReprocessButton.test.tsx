import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeGeometryQuery } from "../../api/queries";
import { ReprocessButton } from "./ReprocessButton";

function renderButton() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });

  return {
    client,
    ...render(
      <QueryClientProvider client={client}>
        <ReprocessButton provider="veloplanner" routeId={12} stageOrder={1} />
      </QueryClientProvider>,
    ),
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("ReprocessButton", () => {
  it("asks the service to redo this stage and no other", async () => {
    const fetchMock = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(JSON.stringify({ status: "accepted" }), { status: 202 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    renderButton();

    await userEvent.click(screen.getByRole("button", { name: "Reprocess" }));

    expect(fetchMock).toHaveBeenCalledWith(
      "/v1/providers/veloplanner/routes/12/stages/1/reprocess",
      expect.objectContaining({ method: "POST" }),
    );
    expect(await screen.findByRole("status")).toHaveTextContent(/Queued/);
  });

  it("says what went wrong rather than looking like it worked", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async (_input: RequestInfo | URL, _init?: RequestInit) =>
          new Response(
            JSON.stringify({ error: { code: "not_found", message: "resource was not found" } }),
            { status: 404 },
          ),
      ),
    );
    renderButton();

    await userEvent.click(screen.getByRole("button", { name: "Reprocess" }));

    expect(await screen.findByRole("status")).toHaveTextContent("resource was not found");
  });

  // Refetching now would fetch the stage as it still is and then hold that
  // answer as fresh for the whole cache window, which is the opposite of what a
  // page waiting for a rewrite needs.
  it("marks the stage's geometry stale without fetching the old one back", async () => {
    const fetchMock = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(JSON.stringify({ status: "accepted" }), { status: 202 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    const { client } = renderButton();
    client.setQueryData(routeGeometryQuery("veloplanner", 12, 1).queryKey, {
      bbox: [8, 49, 8.1, 49.1],
      coordinates: [],
    });

    await userEvent.click(screen.getByRole("button", { name: "Reprocess" }));
    await screen.findByRole("status");

    expect(
      client.getQueryState(routeGeometryQuery("veloplanner", 12, 1).queryKey)?.isInvalidated,
    ).toBe(true);
    expect(fetchMock.mock.calls.some((call) => String(call[0]).includes("/geometry"))).toBe(false);
  });
});
