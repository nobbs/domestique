import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import { statusQuery, webUIConfigQuery } from "../api/queries";
import type { Status, WebUIConfig } from "../api/types";
import { IDLE_STATUS } from "../test/status";
import { MenuBar } from "./MenuBar";

function config(admin: boolean): WebUIConfig {
  return {
    basemaps: [],
    sourceBaseUrls: {},
    timezone: "Europe/Berlin",
    identity: { display: "rider@example.test", admin },
  };
}

function renderBar(admin: boolean, status: Status = IDLE_STATUS) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(webUIConfigQuery().queryKey, config(admin));
  client.setQueryData(statusQuery().queryKey, status);

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <MenuBar />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

/** A run that finished with everything in place, which is the one green state. */
const SYNCED_STATUS: Status = {
  ...IDLE_STATUS,
  sync: { ...IDLE_STATUS.sync, lastCompletedAt: "2026-02-01T09:00:00Z" },
};

const ITEM_WIDTH = 90;
const ITEM_GAP = 4;

function rectOf(left: number, width: number): DOMRect {
  return {
    x: left,
    y: 0,
    width,
    height: 30,
    top: 0,
    right: left + width,
    bottom: 30,
    left,
    toJSON: () => ({}),
  };
}

/**
 * A geometry for jsdom, which lays nothing out and so would report that every
 * name fits in a row of no width at all.
 *
 * Only the two things the arithmetic reads are answered: how wide the row is,
 * and where each item of the measured copy sits. Every name is given the same
 * width, and so is the trailing control — it is the last child of the copy — so
 * a budget of `n * (ITEM_WIDTH + ITEM_GAP)` holds `n - 1` names once the row
 * has overflowed, the control taking the width of the name it stands in for.
 */
function layOutRow(available: number): void {
  vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(function (
    this: Element,
  ): DOMRect {
    if (this.tagName === "NAV") {
      return rectOf(0, available);
    }
    const parent = this.parentElement;

    if (parent?.getAttribute("data-slot") === "overflow-measure") {
      return rectOf([...parent.children].indexOf(this) * (ITEM_WIDTH + ITEM_GAP), ITEM_WIDTH);
    }

    return rectOf(0, 0);
  });
}

describe("the colour scheme", () => {
  // It stands whatever the session is, unlike the pill beside it, which has
  // nothing to say until the configuration arrives.
  it("is offered in the bar, beside the session", () => {
    renderBar(false);

    expect(screen.getByRole("button", { name: /^Theme: / })).toBeInTheDocument();
  });
});

describe("the Admin link", () => {
  it("is offered to an admin", () => {
    renderBar(true);

    expect(screen.getByRole("link", { name: "Admin" })).toHaveAttribute("href", "/admin");
  });

  it("is not offered to a non-admin", () => {
    renderBar(false);

    expect(screen.queryByRole("link", { name: "Admin" })).not.toBeInTheDocument();
  });
});

describe("a row too narrow for every name", () => {
  it("folds the names that do not fit into a menu, and keeps the ones that do", async () => {
    layOutRow(2 * (ITEM_WIDTH + ITEM_GAP));
    renderBar(false);

    expect(screen.getByRole("link", { name: "Atlas" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Settings" })).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "More" }));

    expect(await screen.findByRole("menuitem", { name: "Settings" })).toHaveAttribute(
      "href",
      "/settings",
    );
  });

  /*
   * The dot is the one thing the bar says without being asked, and a fold that
   * hid it would take a signal away exactly where there is least room to go
   * looking for it.
   */
  it("moves the sync state onto the control holding the sync link", () => {
    layOutRow(2 * (ITEM_WIDTH + ITEM_GAP));
    renderBar(false, SYNCED_STATUS);

    expect(screen.getByRole("button", { name: "More" })).toHaveAttribute(
      "title",
      expect.stringContaining("Sync · In sync"),
    );
  });

  /*
   * `aria-current` belongs to the link that is the page, and the control that
   * merely holds it cannot claim to be it — so the emphasis is its own
   * attribute, and a reader whose page has been folded away can still see that
   * it is in there.
   */
  it("marks the control when the page being read is inside it", () => {
    layOutRow(ITEM_WIDTH);
    renderBar(false);

    expect(screen.queryByRole("link", { name: "Atlas" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "More" })).toHaveAttribute("data-holds-current");
  });

  // Every name the width can hold is still a name: the fold is what the row
  // runs out of room for, not a layout it switches to.
  it("folds nothing where the row is wide enough", () => {
    layOutRow(20 * (ITEM_WIDTH + ITEM_GAP));
    renderBar(true);

    expect(screen.getByRole("link", { name: "Settings" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Admin" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "More" })).not.toBeInTheDocument();
  });
});
