import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useNarrowViewport } from "../lib/mediaQuery";

vi.mock("../lib/mediaQuery", () => ({ useNarrowViewport: vi.fn() }));
vi.mock("./MenuBar", () => ({ MenuBar: () => <span>Domestique</span> }));

const { Layout } = await import("./Layout");

function show(
  narrow: boolean,
  labels?: Pick<ComponentProps<typeof Layout>, "drawerLabel" | "drawerTitle" | "workspaceLabel">,
) {
  vi.mocked(useNarrowViewport).mockReturnValue(narrow);

  return render(
    <Layout map={<div aria-label="Route map" role="img" />} {...labels}>
      <button type="button">Route control</button>
    </Layout>,
  );
}

beforeEach(() => {
  vi.mocked(useNarrowViewport).mockReset();
});

describe("Layout", () => {
  it("keeps the route workspace in a non-modal rail on wide screens", () => {
    show(false);

    expect(screen.getByRole("complementary", { name: "Route library controls" })).toContainElement(
      screen.getByRole("button", { name: "Route control" }),
    );
    expect(screen.queryByRole("button", { name: "Browse routes" })).toBeNull();
  });

  // The rail used to bring its own card, which painted an empty rounded square
  // over the map until the first panel mounted and then shed it mid-flight.
  it("gives the overlay rail no card of its own", () => {
    show(false);

    const rail = screen.getByRole("complementary", { name: "Route library controls" });

    expect(rail.className).not.toMatch(/bg-|shadow-|\bp-3\b/);
  });

  it("stands a sidebar workspace and its dock beside the map, off the overlay", () => {
    vi.mocked(useNarrowViewport).mockReturnValue(false);
    render(
      <Layout
        map={<div aria-label="Route map" role="img" />}
        workspace="sidebar"
        dock={<output>Dock</output>}
      >
        <button type="button">Route control</button>
      </Layout>,
    );

    const overlay = document.querySelector(".shell__overlay");
    expect(overlay).not.toContainElement(screen.getByRole("button", { name: "Route control" }));
    expect(overlay).not.toContainElement(screen.getByText("Dock"));
    expect(overlay?.children).toHaveLength(0);
    const pane = screen.getByRole("img", { name: "Route map" }).parentElement?.parentElement;
    expect(pane?.nextElementSibling).toBe(screen.getByText("Dock"));
  });

  it("opens the mobile workspace as a dismissible, labelled Drawer", async () => {
    const user = userEvent.setup();
    show(true);
    const trigger = screen.getByRole("button", { name: "Browse routes" });

    await user.click(trigger);

    expect(screen.getByRole("dialog", { name: "Route library" })).toContainElement(
      screen.getByRole("button", { name: "Route control" }),
    );

    await user.keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Route library" })).toBeNull());
    expect(trigger).toHaveFocus();
  });

  it("uses a page's own labels for the narrow workspace", async () => {
    const user = userEvent.setup();
    show(true, {
      drawerLabel: "Plan a route",
      drawerTitle: "Route planner",
      workspaceLabel: "Route planner controls",
    });
    const trigger = screen.getByRole("button", { name: "Plan a route" });

    await user.click(trigger);

    expect(screen.getByRole("dialog", { name: "Route planner" })).toContainElement(
      screen.getByRole("button", { name: "Route control" }),
    );
  });
});
