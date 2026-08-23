import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { UnitPicker } from "./UnitPicker";

describe("UnitPicker", () => {
  it("names the units on screen, and shows their short mark", () => {
    render(<UnitPicker system="metric" onSystemChange={() => {}} />);

    const toggle = screen.getByRole("button");
    expect(toggle).toHaveTextContent("km");
    expect(toggle).toHaveAccessibleName("Distance and elevation in metric. Switch to imperial.");
  });

  it("names the other system once imperial is on screen", () => {
    render(<UnitPicker system="imperial" onSystemChange={() => {}} />);

    const toggle = screen.getByRole("button");
    expect(toggle).toHaveTextContent("mi");
    expect(toggle).toHaveAccessibleName("Distance and elevation in imperial. Switch to metric.");
  });

  it("asks to switch to the other system on a press", async () => {
    const onSystemChange = vi.fn();
    render(<UnitPicker system="metric" onSystemChange={onSystemChange} />);

    await userEvent.click(screen.getByRole("button"));

    expect(onSystemChange).toHaveBeenCalledWith("imperial");
  });
});
