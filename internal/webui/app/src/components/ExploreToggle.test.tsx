import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ExploreToggle } from "./ExploreToggle";

/**
 * What the control says about itself.
 *
 * The state is read from `aria-pressed` rather than from the label, which is
 * the same in both modes on purpose: a screen reader is told which one the map
 * is in by the toggle's own semantics, and a label that changed underneath that
 * would be the state said twice, in two voices that can disagree.
 */
function toggle(): HTMLElement {
  return screen.getByRole("button", { name: "Explore map" });
}

describe("ExploreToggle", () => {
  it("rests unpressed, with nothing to announce", () => {
    render(<ExploreToggle exploring={false} onExploringChange={() => {}} />);

    expect(toggle()).toHaveAttribute("aria-pressed", "false");
    expect(toggle()).not.toHaveAttribute("aria-keyshortcuts");
    expect(screen.getByText("", { selector: "p" })).toBeEmptyDOMElement();
  });

  it("reads as pressed while the map has the gestures", () => {
    render(<ExploreToggle exploring={true} onExploringChange={() => {}} />);

    expect(toggle()).toHaveAttribute("aria-pressed", "true");
  });

  it("says out loud what the map has taken, and how to give it back", () => {
    render(<ExploreToggle exploring={true} onExploringChange={() => {}} />);

    const note = screen.getByText(/Escape to leave/);
    expect(note).toHaveAttribute("aria-live", "polite");
    expect(toggle()).toHaveAttribute("aria-keyshortcuts", "Escape");
  });

  it("asks for the gestures when it is pressed", async () => {
    const onExploringChange = vi.fn();
    render(<ExploreToggle exploring={false} onExploringChange={onExploringChange} />);
    await userEvent.click(toggle());

    expect(onExploringChange).toHaveBeenCalledWith(true);
  });

  it("gives them back when it is pressed again", async () => {
    const onExploringChange = vi.fn();
    render(<ExploreToggle exploring={true} onExploringChange={onExploringChange} />);
    await userEvent.click(toggle());

    expect(onExploringChange).toHaveBeenCalledWith(false);
  });

  it("is reached and worked by the keyboard alone", async () => {
    const onExploringChange = vi.fn();
    render(<ExploreToggle exploring={false} onExploringChange={onExploringChange} />);

    await userEvent.tab();
    expect(toggle()).toHaveFocus();

    await userEvent.keyboard("{Enter}");
    expect(onExploringChange).toHaveBeenCalledWith(true);

    await userEvent.keyboard(" ");
    expect(onExploringChange).toHaveBeenCalledTimes(2);
  });
});
