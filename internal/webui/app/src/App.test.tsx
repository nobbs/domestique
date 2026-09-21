/**
 * The client routes, read as addresses.
 *
 * A route has its own page at `/routes/…`, so a path naming one renders it
 * directly; only the addresses that predate that page — the bare `/?route=`
 * link and the two-segment path from before a second provider existed — are
 * answered by a redirect. The address each path lands on is the thing worth
 * asserting: it is what a bookmark holds.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routesQuery, statusQuery, webUIConfigQuery } from "./api/queries";
import type { WebUIConfig } from "./api/types";
import { IDLE_STATUS } from "./test/status";

// The pages behind these routes are a WebGL map and a query client; neither is
// what is under test. Standing both in reduces each route to the address it
// resolved to.
vi.mock("./features/routes/AtlasPage", () => ({
  AtlasPage: () => <p>the library</p>,
}));
vi.mock("./features/activity/ActivitiesPage", () => ({
  ActivitiesPage: () => <p>the activities page</p>,
}));
vi.mock("./features/account/AccountPage", () => ({
  AccountPage: () => <p>the account page</p>,
}));
vi.mock("./features/admin/AdminPage", () => ({
  AdminPage: () => <p>the admin page</p>,
}));
vi.mock("./features/plan/PlanPage", () => ({
  PlanPage: () => <p>the planner</p>,
}));

const { App } = await import("./App");

function Address() {
  const { pathname, search } = useLocation();

  return <p data-testid="address">{`${pathname}${search}`}</p>;
}

function config(admin: boolean, planning = false): WebUIConfig {
  return {
    basemaps: [],
    sourceBaseUrls: {},
    timezone: "Europe/Berlin",
    identity: { display: "rider@example.test", admin },
    planning,
  };
}

/**
 * `admin` left undefined leaves the config query unseeded and unfetched, the
 * still-loading state `AdminOnly` must not read as "not admin".
 */
function open(path: string, admin?: boolean, planning = false): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  if (admin !== undefined) {
    client.setQueryData(webUIConfigQuery().queryKey, config(admin, planning));
  }
  // The notice keeps the menu bar, which asks after sync; nothing here is about that.
  client.setQueryData(statusQuery().queryKey, IDLE_STATUS);
  // The menu bar's own ⌘K jump reads the library wherever it is mounted.
  client.setQueryData(routesQuery().queryKey, []);

  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Address />
        <App />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function address(): string {
  return screen.getByTestId("address").textContent ?? "";
}

/**
 * A `localStorage` for jsdom, which has none — see `basemap.test.ts` for why
 * a `Map` behind the two methods the hook uses is enough.
 */
function stubStorage(theme?: string, viewAsRider = false): void {
  const entries = new Map<string, string>();
  if (theme !== undefined) {
    entries.set("domestique.theme", theme);
  }
  if (viewAsRider) {
    entries.set("domestique.viewAsRider", "true");
  }
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => entries.get(key) ?? null,
    setItem: (key: string, value: string) => {
      entries.set(key, value);
    },
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
  document.documentElement.removeAttribute("data-theme");
});

describe("the client routes", () => {
  it("opens a route's own page directly, at its own address", () => {
    open("/routes/veloplanner/12/1", false);

    expect(address()).toBe("/routes/veloplanner/12/1");
    expect(screen.getByText("the library")).toBeInTheDocument();
  });

  // The spelling a link had before providers existed. Only VeloPlanner ever
  // handed one out, so it names that provider and lands on the same route.
  it("answers the two-segment path with the provider it always meant", () => {
    open("/routes/12/1", false);

    expect(address()).toBe("/routes/veloplanner/12/1");
  });

  // The bare address the entry page handed out before a route had a page of
  // its own. Still read, so a link made then still opens the same route.
  it("turns a bare /?route= link into the route's own page", () => {
    open("/?route=veloplanner%2F12%2F1", false);

    expect(address()).toBe("/routes/veloplanner/12/1");
    expect(screen.getByText("the library")).toBeInTheDocument();
  });

  it("reads the two-segment form of that same link the same way", () => {
    open("/?route=12%2F1", false);

    expect(address()).toBe("/routes/veloplanner/12/1");
  });

  it("lands the bare entry address on the rider's activities", () => {
    open("/", false);

    expect(address()).toBe("/activities");
  });

  it("sends anything else to activities too", () => {
    open("/nowhere", false);

    expect(address()).toBe("/activities");
  });

  // The catalogue page is gone; the search palette replaces it.
  it("sends the removed /catalogue to activities", () => {
    open("/catalogue", false);

    expect(address()).toBe("/activities");
  });

  it("serves the account page and each of its tabs", () => {
    open("/account/profile", false);

    expect(address()).toBe("/account/profile");
    expect(screen.getByText("the account page")).toBeInTheDocument();
  });

  // Sync and Settings were merged into Account; their paths were removed, not redirected.
  it.each(["/sync", "/settings", "/settings/tasks"])(
    "sends the removed %s to activities",
    (path) => {
      open(path, true);

      expect(address()).toBe("/activities");
    },
  );

  it("renders the admin page and its tabs for an admin", () => {
    open("/admin/tasks", true);

    expect(address()).toBe("/admin/tasks");
    expect(screen.getByText("the admin page")).toBeInTheDocument();
  });

  it.each(["/admin", "/admin/tasks"])("sends a non-admin from %s to their account", (path) => {
    open(path, false);

    expect(address()).toBe("/account");
  });

  it("mounts the planner only for an admin where routing is configured", () => {
    open("/plan", true, true);

    expect(address()).toBe("/plan");
    expect(screen.getByText("the planner")).toBeInTheDocument();
  });

  it("keeps planner routes absent for a non-admin", () => {
    open("/plan/4", false, true);

    expect(address()).toBe("/activities");
    expect(screen.queryByText("the planner")).not.toBeInTheDocument();
  });

  /**
   * The rider view is read once per module load, so these mount their own copy
   * of the app after the choice is stored — as `identity.test.tsx` does.
   */
  async function openAsPreviewingAdmin(path: string, planning = false) {
    stubStorage(undefined, true);
    vi.resetModules();
    const { App: Fresh } = await import("./App");
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
    });
    client.setQueryData(webUIConfigQuery().queryKey, config(true, planning));
    client.setQueryData(statusQuery().queryKey, IDLE_STATUS);
    client.setQueryData(routesQuery().queryKey, []);
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={[path]}>
          <Address />
          <Fresh />
        </MemoryRouter>
      </QueryClientProvider>,
    );
  }

  it.each(["/admin", "/plan"])(
    "tells an admin previewing the rider view why %s is not shown, and gives it back",
    async (path) => {
      await openAsPreviewingAdmin(path, true);

      expect(address()).toBe(path);
      expect(screen.getByText("Hidden while you view as a rider")).toBeInTheDocument();

      await userEvent.click(screen.getByRole("button", { name: "Leave rider view" }));

      expect(
        screen.getByText(path === "/admin" ? "the admin page" : "the planner"),
      ).toBeInTheDocument();
    },
  );

  // An admin whose service names no routing engine is told that, rather than
  // being bounced as a reader with no business here would be.
  it("says the planner is switched off where no engine is configured", () => {
    stubStorage();
    open("/plan", true, false);

    expect(address()).toBe("/plan");
    expect(screen.getByText("The planner is switched off")).toBeInTheDocument();
  });

  it("explains nothing to a non-admin, who is sent away instead", () => {
    open("/plan", false, true);

    expect(address()).toBe("/activities");
    expect(screen.queryByText(/rider view/)).not.toBeInTheDocument();
    expect(screen.queryByText(/planner is switched off/)).not.toBeInTheDocument();
  });

  // Deciding before the caller's own identity has arrived would bounce an
  // admin on first paint; nothing is rendered until it settles.
  it("renders nothing at /admin while identity is still loading", () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise<Response>(() => {})),
    );
    open("/admin");

    expect(address()).toBe("/admin");
    expect(screen.queryByText("the admin page")).not.toBeInTheDocument();
    expect(screen.queryByText("the account page")).not.toBeInTheDocument();
  });
});

/*
 * `data-theme` is what `index.css`'s explicit-override blocks key off — see
 * there for why. It is a document-level attribute rather than something a
 * page's own markup carries, so it is asserted here, at the one place that
 * applies it regardless of which page is mounted.
 */
describe("the document theme", () => {
  it("sets no override for the system default", () => {
    stubStorage();
    open("/routes/veloplanner/12/1", false);

    expect(document.documentElement.hasAttribute("data-theme")).toBe(false);
  });

  it("applies the reader's remembered override on load", () => {
    stubStorage("dark");
    open("/routes/veloplanner/12/1", false);

    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });
});
