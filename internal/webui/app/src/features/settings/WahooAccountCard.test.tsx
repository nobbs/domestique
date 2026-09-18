import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { statusQuery, webUIConfigQuery } from "../../api/queries";
import type { Status, WebUIConfig } from "../../api/types";
import { WahooAccountCard } from "./WahooAccountCard";

afterEach(() => {
  vi.unstubAllGlobals();
});

const STATUS: Status = {
  ready: true,
  converged: true,
  targets: [
    {
      id: "rider-a",
      authorisation: "authorized",
      convergence: "current",
      routes: { current: 4, pending: 0 },
    },
  ],
  sync: {
    state: "idle",
    sourceRoutes: 4,
    created: 0,
    updated: 0,
    deleted: 0,
    phases: {},
    surface: { classified: 0, total: 0, incomplete: 0, enrichmentFailures: 0 },
  },
};

function renderCard(answer: number) {
  const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) =>
    init?.method === "DELETE"
      ? new Response(null, { status: answer })
      : new Response(JSON.stringify(STATUS), { status: 200 }),
  );
  vi.stubGlobal("fetch", fetchMock);
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
      mutations: { retry: false },
    },
  });
  client.setQueryData(statusQuery().queryKey, STATUS);
  client.setQueryData(webUIConfigQuery().queryKey, {
    basemaps: [],
    sourceBaseUrls: {},
    timezone: "Europe/Berlin",
    identity: { display: "rider@example.test", admin: false },
  } as WebUIConfig);
  render(
    <QueryClientProvider client={client}>
      <WahooAccountCard />
    </QueryClientProvider>,
  );

  return fetchMock;
}

describe("WahooAccountCard", () => {
  it("offers only a disconnect, and sends it for the caller's own account", async () => {
    const fetchMock = renderCard(204);
    expect(screen.queryByRole("button", { name: /^Reconcile now/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Delete all routes…" })).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Disconnect Wahoo account" }));

    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some(
          ([url, init]) =>
            url === "/v1/settings/rider/connections/wahoo" && init?.method === "DELETE",
        ),
      ).toBe(true),
    );
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("says when the account stayed connected", async () => {
    renderCard(502);

    await userEvent.click(screen.getByRole("button", { name: "Disconnect Wahoo account" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Your Wahoo account was not disconnected.",
    );
  });
});
