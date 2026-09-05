import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { OverlayToggle } from "./OverlayToggle";

describe("OverlayToggle", () => {
  it("reports its state and flips it on press", () => {
    const onChange = vi.fn();
    render(
      <OverlayToggle on={false} onChange={onChange} icon={<span />} subject="wind" title="Wind" />,
    );
    const button = screen.getByRole("button", { name: "Show the wind over the map" });
    expect(button).toHaveAttribute("aria-pressed", "false");
    fireEvent.click(button);
    expect(onChange).toHaveBeenCalledWith(true);
  });
});
