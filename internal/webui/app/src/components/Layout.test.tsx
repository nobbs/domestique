import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useNarrowViewport } from "../lib/mediaQuery";

vi.mock("../lib/mediaQuery", () => ({ useNarrowViewport: vi.fn() }));
vi.mock("./MenuBar", () => ({ MenuBar: () => <span>Domestique</span> }));

const { Layout } = await import("./Layout");

function show(narrow: boolean) {
  vi.mocked(useNarrowViewport).mockReturnValue(narrow);

  return render(
    <Layout map={<div aria-label="Route map" role="img" />}>
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
});
