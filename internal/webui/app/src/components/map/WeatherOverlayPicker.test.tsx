import { IconTemperature, IconWind } from "@tabler/icons-react";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
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

  it("shows the hour scale whether or not a measure is on", () => {
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
    // Only the prefix this component itself writes, never the formatted
    // weekday and time after it: `toLocaleTimeString` renders those however
    // the runtime's own locale does, which is not this component's to assert.
    expect(screen.getByText(/^Now · /)).toBeInTheDocument();
    // Screen-reader access, not just sight: a slider whose thumb carries no
    // name of its own announces as "slider", unlabelled, on every platform
    // that does not happen to render the sighted layout beside it.
    expect(screen.getByRole("slider", { name: /^When, /u })).toBeInTheDocument();

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
    expect(screen.getByText(/^\+3h · /)).toBeInTheDocument();
  });

  it("asks for the weekday in the hour label, whatever script or order the runtime renders it in", () => {
    // The label's own promise is that a reader scrubbed past midnight can
    // still tell which day they are looking at — a promise the DOM text
    // cannot check without assuming a locale, so this checks the request
    // the component makes instead of the locale-dependent string it gets back.
    const spy = vi.spyOn(Date.prototype, "toLocaleTimeString");
    render(
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

    expect(spy).toHaveBeenCalledWith(undefined, expect.objectContaining({ weekday: "short" }));
    spy.mockRestore();
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

  describe("the hour label over time", () => {
    beforeEach(() => {
      vi.useFakeTimers();
      vi.setSystemTime(new Date("2026-09-05T12:59:00Z"));
    });

    afterEach(() => {
      vi.useRealTimers();
    });

    it("re-reads the clock on its own once an hour passes with a measure on", () => {
      const spy = vi.spyOn(Date.prototype, "toLocaleTimeString");
      render(
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
      const callsBefore = spy.mock.calls.length;

      act(() => {
        vi.advanceTimersByTime(70 * 60_000);
      });

      expect(spy.mock.calls.length).toBeGreaterThan(callsBefore);
      spy.mockRestore();
    });

    it("does not keep ticking while nothing is on", () => {
      const spy = vi.spyOn(Date.prototype, "toLocaleTimeString");
      render(
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
      const callsBefore = spy.mock.calls.length;

      vi.advanceTimersByTime(70 * 60_000);

      expect(spy.mock.calls.length).toBe(callsBefore);
      spy.mockRestore();
    });
  });
});
