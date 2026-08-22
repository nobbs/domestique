/**
 * The client routes, read as addresses.
 *
 * A route is a panel over the library rather than a page of its own, so every
 * path that names one is answered by a redirect into the query the library
 * reads. The address each path lands on is the thing worth asserting: it is what
 * a bookmark holds, and both spellings of it have to keep naming the same stage
 * now that a stage's identity carries its provider.
 */

import { render, screen } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router";
import { describe, expect, it, vi } from "vitest";

// The pages behind these routes are a WebGL map and a query client; neither is
// what is under test. Standing both in reduces each route to the address it
// resolved to.
vi.mock("./features/routes/RoutesPage", () => ({
  RoutesPage: () => <p>the library</p>,
}));
vi.mock("./features/sync/SyncPage", () => ({
  SyncPage: () => <p>the sync page</p>,
}));

const { App } = await import("./App");

function Address() {
  const { pathname, search } = useLocation();

  return <p data-testid="address">{`${pathname}${search}`}</p>;
}

function open(path: string): void {
  render(
    <MemoryRouter initialEntries={[path]}>
      <Address />
      <App />
    </MemoryRouter>,
  );
}

function address(): string {
  return screen.getByTestId("address").textContent ?? "";
}

describe("the client routes", () => {
  it("turns a route's path into the query the library opens it from", () => {
    open("/routes/veloplanner/12/1");

    expect(address()).toBe("/?route=veloplanner%2F12%2F1");
    expect(screen.getByText("the library")).toBeInTheDocument();
  });

  // The spelling a link had before providers existed. Only VeloPlanner ever
  // handed one out, so it names that provider and lands on the same stage.
  it("answers the two-segment path with the provider it always meant", () => {
    open("/routes/12/1");

    expect(address()).toBe("/?route=veloplanner%2F12%2F1");
  });

  it("sends anything else back to the library", () => {
    open("/nowhere");

    expect(address()).toBe("/");
  });

  it("keeps sync a page of its own", () => {
    open("/sync");

    expect(address()).toBe("/sync");
    expect(screen.getByText("the sync page")).toBeInTheDocument();
  });
});
