/**
 * The client routes, read as addresses.
 *
 * A route is a panel over the library rather than a page of its own, so every
 * path that names one is answered by a redirect into the query the library
 * reads. The address each path lands on is the thing worth asserting: it is what
 * a bookmark holds, and both spellings of it have to keep naming the same route
 * now that a route's identity carries its provider.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { webUIConfigQuery } from "./api/queries";
import type { WebUIConfig } from "./api/types";

// The pages behind these routes are a WebGL map and a query client; neither is
// what is under test. Standing both in reduces each route to the address it
// resolved to.
vi.mock("./features/routes/AtlasPage", () => ({
  AtlasPage: () => <p>the library</p>,
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
function stubStorage(theme?: string): void {
  const entries = new Map<string, string>();
  if (theme !== undefined) {
    entries.set("domestique.theme", theme);
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
  it("turns a route's path into the query the library opens it from", () => {
    open("/routes/veloplanner/12/1");

    expect(address()).toBe("/?route=veloplanner%2F12%2F1");
    expect(screen.getByText("the library")).toBeInTheDocument();
  });

  // The spelling a link had before providers existed. Only VeloPlanner ever
  // handed one out, so it names that provider and lands on the same route.
  it("answers the two-segment path with the provider it always meant", () => {
    open("/routes/12/1");

    expect(address()).toBe("/?route=veloplanner%2F12%2F1");
  });

  it("sends anything else back to the library", () => {
    open("/nowhere");

    expect(address()).toBe("/");
  });

  it("serves the account page and each of its tabs", () => {
    open("/account/profile");

    expect(address()).toBe("/account/profile");
    expect(screen.getByText("the account page")).toBeInTheDocument();
  });

  // Sync and Settings were merged into Account; their paths were removed, not redirected.
  it.each(["/sync", "/settings", "/settings/tasks"])(
    "sends the removed %s to the library",
    (path) => {
      open(path, true);

      expect(address()).toBe("/");
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

    expect(address()).toBe("/");
    expect(screen.queryByText("the planner")).not.toBeInTheDocument();
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
    open("/");

    expect(document.documentElement.hasAttribute("data-theme")).toBe(false);
  });

  it("applies the reader's remembered override on load", () => {
    stubStorage("dark");
    open("/");

    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });
});
