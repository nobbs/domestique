import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SyncPage } from "./SyncPage";

function statusBody() {
  return {
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
      last_completed_at: "2026-08-18T06:30:00Z",
      source_stages: 4,
      created: 0,
      updated: 0,
      deleted: 0,
      schedule: { source: true, targets: true },
      phases: {},
      surface: { classified: 4, total: 4, incomplete: 0 },
    },
  };
}

/**
 * Every request the page makes, answered from one place. The page is four
 * cards that each fetch for themselves, so a test that stubbed only one of them
 * would be asserting against a page half of which had failed.
 */
function renderPage(path = "/sync", body: unknown = statusBody()) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.startsWith("/v1/sync/runs")) {
        return new Response(JSON.stringify({ runs: [] }), { status: 200 });
      }
      if (url.startsWith("/v1/tasks")) {
        return new Response(JSON.stringify({ tasks: [] }), { status: 200 });
      }

      return new Response(JSON.stringify(body), { status: 200 });
    }),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <SyncPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("SyncPage", () => {
  it("puts the operational questions in scan order under the bar that leads back", async () => {
    renderPage();

    expect(screen.getByRole("heading", { level: 1, name: "Sync" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Atlas" })).toHaveAttribute("href", "/");
    const headings = (await screen.findAllByRole("heading", { level: 2 })).map(
      (heading) => heading.textContent,
    );
    expect(headings).toEqual(["Now", "What the targets hold", "What has happened"]);
    expect(screen.queryByRole("radio", { name: "Metric (km)" })).toBeNull();
  });

  // A notification carries one opaque reference and lands here. A reference for
  // a run the history no longer holds is the pruning working, and the page says
  // so rather than showing nothing.
  it("reads the run a notification named out of the address", async () => {
    renderPage("/sync?run=aaaaaaaaaaaa");

    expect(
      await screen.findByRole("heading", { name: "That run is no longer kept" }),
    ).toBeInTheDocument();
  });

  // `/sync?run=` names no run. Reading the empty string as a reference would put
  // a card at the top of the page about a run nobody asked after.
  it("ignores a run parameter with nothing in it", async () => {
    renderPage("/sync?run=");

    // Waited on so the assertion below is made against a settled history rather
    // than against a card that had not decided what to say yet.
    expect(await screen.findByText("Nothing has run yet.")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /That run/ })).toBeNull();
  });
});
