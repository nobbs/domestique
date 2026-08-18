import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ReprocessButton } from "./ReprocessButton";

function renderButton() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });

  return render(
    <QueryClientProvider client={client}>
      <ReprocessButton routeId={12} stageOrder={1} />
    </QueryClientProvider>,
  );
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
      "/v1/routes/12/stages/1/reprocess",
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
});
