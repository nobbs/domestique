import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { SwitchableView } from "./SwitchableView";

const VIEWS = [
  { value: "ring", label: "Ring", content: () => <p>the ring</p> },
  { value: "bars", label: "Bars", content: () => <p>the bars</p> },
] as const;

describe("SwitchableView", () => {
  it("shows the first view, and only that one, until another is picked", () => {
    render(<SwitchableView label="View" views={VIEWS} />);

    expect(screen.getByText("the ring")).toBeInTheDocument();
    expect(screen.queryByText("the bars")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Ring" })).toHaveAttribute("aria-pressed", "true");
  });

  it("swaps to the picked view and says which one it was", async () => {
    const onValueChange = vi.fn();
    render(<SwitchableView label="View" views={VIEWS} onValueChange={onValueChange} />);

    await userEvent.click(screen.getByRole("button", { name: "Bars" }));

    expect(screen.getByText("the bars")).toBeInTheDocument();
    expect(screen.queryByText("the ring")).not.toBeInTheDocument();
    expect(onValueChange).toHaveBeenCalledWith("bars");
  });

  it("keeps a view shown when its own pressed button is pressed again", async () => {
    render(<SwitchableView label="View" views={VIEWS} defaultValue="bars" />);

    await userEvent.click(screen.getByRole("button", { name: "Bars" }));

    expect(screen.getByText("the bars")).toBeInTheDocument();
  });

  it("offers no switch when there is only one way to show it", () => {
    render(<SwitchableView label="View" heading={<h3>Heading</h3>} views={VIEWS.slice(0, 1)} />);

    expect(screen.getByRole("heading", { name: "Heading" })).toBeInTheDocument();
    expect(screen.queryByRole("group", { name: "View" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});
