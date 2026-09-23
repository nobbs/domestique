import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { riderProfileQuery, statusQuery, webUIConfigQuery } from "../../api/queries";
import type { Status, TargetStatus, WebUIConfig } from "../../api/types";
import { AccountPage } from "./AccountPage";

function target(overrides: Partial<TargetStatus> = {}): TargetStatus {
  return {
    id: "rider-a",
    authorisation: "authorized",
    convergence: "current",
    routes: { current: 4, pending: 0 },
    ...overrides,
  };
}

function status(targets: TargetStatus[]): Status {
  return {
    ready: true,
    converged: true,
    targets,
    sync: {
      state: "idle",
      sourceRoutes: 0,
      created: 0,
      updated: 0,
      deleted: 0,
      phases: {},
      surface: { classified: 0, total: 0, incomplete: 0, enrichmentFailures: 0 },
    },
  };
}

function config(admin = false): WebUIConfig {
  return {
    basemaps: [],
    sourceBaseUrls: {},
    timezone: "Europe/Berlin",
    identity: { display: "rider@example.test", admin },
  };
}

function Address() {
  const { pathname, search } = useLocation();
  return <p data-testid="address">{`${pathname}${search}`}</p>;
}

/** Every card fetches for itself; the ones not seeded are answered empty here. */
function renderPage(path: string, targets: TargetStatus[] = [target()], admin = false) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      const body = url.startsWith("/v1/sync/runs")
        ? { runs: [] }
        : url.startsWith("/v1/tasks")
          ? { tasks: [] }
          : {};
      return new Response(JSON.stringify(body), { status: 200 });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(statusQuery().queryKey, status(targets));
  client.setQueryData(webUIConfigQuery().queryKey, config(admin));
  client.setQueryData(riderProfileQuery().queryKey, {
    profile: {},
    suggestions: {},
    zwift: { emailSet: false, passwordSet: false },
    wahoo: { emailSet: false, passwordSet: false, signInRefused: false },
  });

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Address />
        <Routes>
          <Route path="account" element={<AccountPage />} />
          <Route path="account/:section" element={<AccountPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const address = () => screen.getByTestId("address").textContent;

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("AccountPage", () => {
  it("opens on the sync tab, with its cards in scan order", async () => {
    renderPage("/account");

    expect(address()).toBe("/account/sync");
    expect(screen.getByRole("heading", { level: 1, name: "Account" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Sync" })).toHaveAttribute("aria-selected", "true");
    const headings = (await screen.findAllByRole("heading", { level: 2 })).map(
      (heading) => heading.textContent,
    );
    expect(headings).toEqual(["Now", "What the targets hold", "What has happened"]);
  });

  it("moves to a tab's own address when it is chosen", async () => {
    renderPage("/account/sync");

    await userEvent.click(screen.getByRole("tab", { name: "Rider profile" }));

    expect(address()).toBe("/account/profile");
  });

  it("lands an unknown tab on the first, keeping the query", () => {
    renderPage("/account/nowhere?run=aaaaaaaaaaaa");

    expect(address()).toBe("/account/sync?run=aaaaaaaaaaaa");
  });

  // A notification carries one opaque reference; a run the history no longer
  // holds is the pruning working, and the tab says so.
  it("reads the run a notification named out of the address", async () => {
    renderPage("/account/sync?run=aaaaaaaaaaaa");

    expect(
      await screen.findByRole("heading", { name: "That run is no longer kept" }),
    ).toBeInTheDocument();
  });

  it("ignores a run parameter with nothing in it", async () => {
    renderPage("/account/sync?run=");

    expect(await screen.findByText("Nothing has run yet.")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /That run/ })).toBeNull();
  });

  it("shows the caller's own connected target on the accounts tab", () => {
    renderPage("/account/accounts");

    expect(screen.getByText("rider-a")).toBeInTheDocument();
  });

  it("offers the connect flow when the caller has no target yet", () => {
    renderPage("/account/accounts", []);

    expect(screen.getByRole("link", { name: "Connect it" })).toBeInTheDocument();
  });

  // An admin's fleet is in slot order; `own` says which target is theirs.
  it("shows the admin's own target even when it is not first in the fleet", () => {
    renderPage(
      "/account/accounts",
      [
        target({ id: "rider-b", owner: "rider-b" }),
        target({ id: "rider-a", owner: "admin", own: true }),
      ],
      true,
    );

    expect(screen.getByText("rider-a")).toBeInTheDocument();
    expect(screen.queryByText("rider-b")).not.toBeInTheDocument();
  });

  it("keeps the shared service settings off every tab", () => {
    renderPage("/account/accounts");

    expect(screen.queryByText("Wahoo application")).not.toBeInTheDocument();
  });
});
