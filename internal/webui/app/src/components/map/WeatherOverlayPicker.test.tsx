import { IconTemperature, IconWind } from "@tabler/icons-react";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { WeatherOverlayPicker } from "./WeatherOverlayPicker";

const MEASURES = [
  { key: "wind" as const, label: "Wind", icon: IconWind },
  { key: "temperature" as const, label: "Temperature", icon: IconTemperature },
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

  it("shows the hour scale whether or not a measure is on, with the weekday in its label", () => {
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
    expect(screen.getByText(/^Now · \p{L}+ \d/u)).toBeInTheDocument();

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
    expect(screen.getByText(/^\+3h · \p{L}+ \d/u)).toBeInTheDocument();
  });

  it("disables the hour scale until a measure is on", () => {
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
    expect(screen.getByRole("slider")).toBeDisabled();

    rerender(
      <WeatherOverlayPicker
        measures={MEASURES}
        selected={new Set(["wind"])}
        onToggle={vi.fn()}
        hoursAhead={0}
        onHoursAheadChange={vi.fn()}
        expanded={true}
        onExpandedChange={vi.fn()}
      />,
    );
    expect(screen.getByRole("slider")).toBeEnabled();
  });
});
