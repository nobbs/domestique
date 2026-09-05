import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { WeatherOverlayPicker } from "./WeatherOverlayPicker";

const MEASURES = [
  { key: "wind" as const, label: "Wind" },
  { key: "temperature" as const, label: "Temperature" },
];

describe("WeatherOverlayPicker", () => {
  it("opens to show a checkbox per measure and toggles it", () => {
    const onToggle = vi.fn();
    render(
      <WeatherOverlayPicker
        measures={MEASURES}
        selected={new Set()}
        onToggle={onToggle}
        hoursAhead={0}
        onHoursAheadChange={vi.fn()}
        expanded={true}
        onExpandedChange={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("checkbox", { name: "Wind" }));
    expect(onToggle).toHaveBeenCalledWith("wind", true);
  });

  it("hides the hour scale until something is on, then reads it out", () => {
    const { rerender } = render(
      <WeatherOverlayPicker
        measures={MEASURES}
        selected={new Set()}
        onToggle={vi.fn()}
        hoursAhead={0}
        onHoursAheadChange={vi.fn()}
        expanded={true}
        onExpandedChange={vi.fn()}
      />,
    );
    expect(screen.queryByText("Now")).not.toBeInTheDocument();

    rerender(
      <WeatherOverlayPicker
        measures={MEASURES}
        selected={new Set(["wind"])}
        onToggle={vi.fn()}
        hoursAhead={3}
        onHoursAheadChange={vi.fn()}
        expanded={true}
        onExpandedChange={vi.fn()}
      />,
    );
    expect(screen.getByText(/^\+3h/)).toBeInTheDocument();
  });
});
