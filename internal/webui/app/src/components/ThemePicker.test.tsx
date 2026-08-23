/**
 * The theme chooser, folded and unfolded.
 *
 * Mirrors `BasemapPicker.test.tsx`: one named affordance whose state is
 * reported rather than drawn, a group of radios behind it, and exactly one of
 * them checked.
 */

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import type { ThemeChoice } from "../lib/theme";
import { ThemePicker } from "./ThemePicker";

/** Holds the fold, the same reason `BasemapPicker.test.tsx`'s `Held` does. */
function Held({
  choice = "system",
  onChoose = () => {},
}: {
  choice?: ThemeChoice;
  onChoose?: (choice: ThemeChoice) => void;
}) {
  const [expanded, setExpanded] = useState(false);

  return (
    <ThemePicker
      choice={choice}
      onChoose={onChoose}
      expanded={expanded}
      onExpandedChange={setExpanded}
    />
  );
}

/** Unfolds the list, which is where everything below is. */
async function open() {
  await userEvent.click(screen.getByRole("button", { name: "Choose the colour theme" }));
}

describe("ThemePicker", () => {
  it("keeps the choices folded away until they are asked for", () => {
    render(<Held />);

    expect(screen.queryByRole("radiogroup")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Choose the colour theme" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
  });

  it("offers system, light and dark, in that order, once unfolded", async () => {
    render(<Held />);
    await open();

    const group = screen.getByRole("radiogroup", { name: "Colour theme" });
    expect(group).toBeInTheDocument();
    expect(screen.getAllByRole("radio").map((radio) => radio.getAttribute("value"))).toEqual([
      "system",
      "light",
      "dark",
    ]);
  });

  /*
   * The name is the whole of what the button says, so it has to change with
   * the fold — the same reasoning `BasemapPicker` documents for its own
   * toggle.
   */
  it("names what pressing the mark would do, either way", async () => {
    render(<Held />);
    await open();

    const toggle = screen.getByRole("button", { name: "Hide the colour theme choices" });
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    expect(toggle).toHaveAttribute("aria-controls", screen.getByRole("radiogroup").id);
  });

  it("marks the reader's own choice, and only that one", async () => {
    render(<Held choice="dark" />);
    await open();

    expect(screen.getByRole("radio", { name: "Dark" })).toBeChecked();
    expect(screen.getByRole("radio", { name: "System" })).not.toBeChecked();
    expect(screen.getByRole("radio", { name: "Light" })).not.toBeChecked();
  });

  it("reports a pick", async () => {
    const onChoose = vi.fn();
    render(<Held onChoose={onChoose} />);
    await open();

    await userEvent.click(screen.getByRole("radio", { name: "Light" }));

    expect(onChoose).toHaveBeenCalledWith("light");
  });
});
