import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { webUIConfigQuery } from "../api/queries";
import type { WebUIConfig } from "../api/types";
import { initialsOf, UserPill } from "./UserPill";

function config(admin: boolean): WebUIConfig {
  return {
    basemaps: [],
    sourceBaseUrls: {},
    timezone: "Europe/Berlin",
    identity: { display: "rider@example.test", admin },
  };
}

function renderPill(admin: boolean) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(webUIConfigQuery().queryKey, config(admin));

  return render(
    <QueryClientProvider client={client}>
      <UserPill />
    </QueryClientProvider>,
  );
}

/** A `localStorage` for jsdom, which has none. See `basemap.test.ts` for why a `Map` behind the two methods the hook uses is enough. */
function stubStorage(): void {
  const entries = new Map<string, string>();
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => entries.get(key) ?? null,
    setItem: (key: string, value: string) => {
      entries.set(key, value);
    },
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("the view-as-rider switch", () => {
  it("is offered to an admin", async () => {
    stubStorage();
    renderPill(true);

    await userEvent.click(screen.getByRole("button", { name: /Signed in as/ }));

    expect(
      await screen.findByRole("menuitemcheckbox", { name: "View as rider" }),
    ).toBeInTheDocument();
  });

  it("is not offered to a non-admin", async () => {
    stubStorage();
    renderPill(false);

    await userEvent.click(screen.getByRole("button", { name: /Signed in as/ }));

    expect(await screen.findByRole("menu")).toBeInTheDocument();
    expect(
      screen.queryByRole("menuitemcheckbox", { name: "View as rider" }),
    ).not.toBeInTheDocument();
  });

  it("persists the flip to localStorage", async () => {
    stubStorage();
    renderPill(true);

    await userEvent.click(screen.getByRole("button", { name: /Signed in as/ }));
    const toggle = await screen.findByRole("menuitemcheckbox", { name: "View as rider" });
    await userEvent.click(toggle);

    expect(window.localStorage.getItem("domestique.viewAsRider")).toBe("true");
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
