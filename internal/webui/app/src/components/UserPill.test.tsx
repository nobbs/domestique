import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { statusQuery, webUIConfigQuery } from "../api/queries";
import type { Status, TargetStatus, WebUIConfig } from "../api/types";
import { IDLE_STATUS } from "../test/status";
import { initialsOf, UserPill } from "./UserPill";

function config(admin: boolean, impersonating = false): WebUIConfig {
  return {
    basemaps: [],
    sourceBaseUrls: {},
    timezone: "Europe/Berlin",
    identity: { display: "rider@example.test", admin, ...(impersonating ? { impersonating } : {}) },
  };
}

function renderPill(admin: boolean, status: Status = IDLE_STATUS, impersonating = false) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(webUIConfigQuery().queryKey, config(admin, impersonating));
  client.setQueryData(statusQuery().queryKey, status);

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <UserPill />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("the account link", () => {
  it("is in the session menu", async () => {
    renderPill(false);

    await userEvent.click(screen.getByRole("button", { name: /Signed in as/ }));

    expect(await screen.findByRole("menuitem", { name: /^Account/ })).toHaveAttribute(
      "href",
      "/account",
    );
  });

  // Sync's state moved with its page: the dot rides on the session, and the
  // item's name says what it meant.
  it("carries what sync is doing onto the avatar and the item", async () => {
    const unconnected: TargetStatus = {
      id: "rider-a",
      authorisation: "not_authorized",
      convergence: "unauthorized",
      routes: { current: 0, pending: 4 },
    };
    renderPill(false, { ...IDLE_STATUS, targets: [unconnected] });

    const trigger = screen.getByRole("button", { name: /Signed in as/ });
    expect(trigger).toHaveAttribute("data-tone", "alert");
    await userEvent.click(trigger);

    expect(
      await screen.findByRole("menuitem", { name: "Account · Sync · A target is not connected" }),
    ).toBeInTheDocument();
  });
});

const TARGET: TargetStatus = {
  id: "admin",
  authorisation: "authorized",
  convergence: "current",
  routes: { current: 0, pending: 0 },
};
const RIDERS: Status = {
  ...IDLE_STATUS,
  targets: [
    { ...TARGET, owner: "admin", own: true },
    { ...TARGET, id: "rider-b", owner: "rider-b", own: false, ownerNickname: "Bee" },
  ],
};

/** A `fetch` that answers every request with 204 and records it. */
function stubOkFetch() {
  const calls = vi.fn((_input: RequestInfo | URL, _init?: RequestInit) =>
    Promise.resolve(new Response(null, { status: 204 })),
  );
  vi.stubGlobal("fetch", calls);

  return calls;
}

describe("impersonation", () => {
  it("offers an admin every other rider, and asks to view as the one picked", async () => {
    const calls = stubOkFetch();
    renderPill(true, RIDERS);

    await userEvent.click(screen.getByRole("button", { name: /Signed in as/ }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "View as…" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Bee" }));

    expect(screen.queryByRole("menuitem", { name: "admin" })).not.toBeInTheDocument();
    const [input, init] = calls.mock.calls[0] ?? [];
    expect(input).toBe("/auth/impersonate");
    expect(String(init?.body)).toBe("subject=rider-b");
  });

  it("is not offered to a non-admin", async () => {
    renderPill(false, RIDERS);

    await userEvent.click(screen.getByRole("button", { name: /Signed in as/ }));

    expect(await screen.findByRole("menu")).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "View as…" })).not.toBeInTheDocument();
  });

  it("says whose session this is while impersonating, and offers the way back", async () => {
    const calls = stubOkFetch();
    renderPill(false, IDLE_STATUS, true);

    await userEvent.click(screen.getByRole("button", { name: "Viewing as rider@example.test" }));
    await userEvent.click(
      await screen.findByRole("menuitem", { name: "Stop viewing as rider@example.test" }),
    );

    expect(calls.mock.calls[0]?.[0]).toBe("/auth/impersonate/stop");
  });
});

describe("initialsOf", () => {
  it("takes the local part, because every address shares its domain", () => {
    expect(initialsOf("rider@example.test")).toBe("R");
  });

  // A separator in the local part usually parts a given name from a family
  // one, and two initials tell two addresses apart where the first two letters
  // of one name would not.
  it("reads a separated local part as two names", () => {
    expect(initialsOf("alexej.disterhoft@example.test")).toBe("AD");
    expect(initialsOf("jean-luc@example.test")).toBe("JL");
    expect(initialsOf("a_b@example.test")).toBe("AB");
    expect(initialsOf("rider+wahoo@example.test")).toBe("RW");
  });

  // The provider hands over a name when the account has one and no email, so
  // the display value is not always an address.
  it("reads a plain name as two names", () => {
    expect(initialsOf("Demo Rider")).toBe("DR");
    expect(initialsOf("Rider")).toBe("R");
  });

  it("takes only the first two, however many parts there are", () => {
    expect(initialsOf("one.two.three.four@example.test")).toBe("OT");
  });

  it("does not mistake a repeated separator for a name", () => {
    expect(initialsOf("first..last@example.test")).toBe("FL");
  });

  /*
   * The gate will not admit an address the service was not configured with, so
   * none of these can arrive from it. They are here because the value crosses
   * the wire, and a corner of the bar is a poor place to find that out.
   */
  it("says nothing rather than throwing on an address that is not one", () => {
    expect(initialsOf("")).toBe("");
    expect(initialsOf("@example.test")).toBe("");
    expect(initialsOf("  rider@example.test  ")).toBe("R");
  });
});
