import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { WindOverlayToggle } from "./WindOverlayToggle";

describe("WindOverlayToggle", () => {
  it("reports its state and flips it on press", () => {
    const onChange = vi.fn();
    render(<WindOverlayToggle on={false} onChange={onChange} />);
    const button = screen.getByRole("button", { name: "Show the wind over the map" });
    expect(button).toHaveAttribute("aria-pressed", "false");
    fireEvent.click(button);
    expect(onChange).toHaveBeenCalledWith(true);
  });
});
