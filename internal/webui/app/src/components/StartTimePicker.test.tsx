import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StartTimePicker } from "./StartTimePicker";

const NOW = new Date("2026-08-24T12:00:00Z");

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(NOW);
});

afterEach(() => {
  vi.useRealTimers();
});

function input(): HTMLInputElement {
  return screen.getByLabelText("Ride start") as HTMLInputElement;
}

/** What a `datetime-local` field carries for a moment: local time, to the minute. */
function localInputValue(date: Date): string {
  const pad = (value: number) => String(value).padStart(2, "0");

  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

describe("StartTimePicker", () => {
  it("opens empty when nothing has been chosen", () => {
    render(<StartTimePicker value={null} onChange={() => {}} />);

    expect(input().value).toBe("");
  });

  it("accepts a time inside the forecast window", () => {
    const onChange = vi.fn();
    render(<StartTimePicker value={null} onChange={onChange} />);

    // The morning after "now": comfortably inside both bounds of the window.
    fireEvent.change(input(), { target: { value: "2026-08-25T07:00" } });

    expect(onChange).toHaveBeenCalledWith(new Date("2026-08-25T07:00"));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("refuses a time more than a day in the past, and says so", () => {
    const onChange = vi.fn();
    render(<StartTimePicker value={null} onChange={onChange} />);

    fireEvent.change(input(), { target: { value: "2026-08-01T07:00" } });

    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toHaveTextContent("more than a day in the past");
  });

  it("refuses a time beyond the forecast horizon, and says so", () => {
    const onChange = vi.fn();
    render(<StartTimePicker value={null} onChange={onChange} />);

    fireEvent.change(input(), { target: { value: "2026-10-01T07:00" } });

    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toHaveTextContent("forecast horizon");
  });

  /*
   * The forecast request spans the whole ride, so the horizon belongs to the
   * arrival rather than the departure. A start the endpoint would refuse comes
   * back as a 400 the page can only report as the provider being unavailable,
   * which is a lie about whose fault it was.
   */
  it("refuses a start whose finish would fall past the horizon", () => {
    const onChange = vi.fn();
    const sixHours = 6 * 60 * 60;
    render(<StartTimePicker value={null} onChange={onChange} movingSeconds={sixHours} />);

    // Just inside the 16-day window at the start line, and past it at the finish.
    const nearlyTheHorizon = new Date(Date.now() + 16 * 24 * 60 * 60 * 1000 - 60 * 60 * 1000);
    fireEvent.change(input(), {
      target: { value: nearlyTheHorizon.toISOString().slice(0, 16) },
    });

    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toHaveTextContent("forecast horizon");
  });

  it("offers a later start for a short ride than for a long one", () => {
    const { unmount } = render(
      <StartTimePicker value={null} onChange={() => {}} movingSeconds={60 * 60} />,
    );
    const shortRideMax = input().getAttribute("max") ?? "";
    unmount();

    render(<StartTimePicker value={null} onChange={() => {}} movingSeconds={10 * 60 * 60} />);

    expect(input().getAttribute("max") ?? "").not.toBe("");
    expect((input().getAttribute("max") ?? "") < shortRideMax).toBe(true);
  });

  /*
   * The bounds are drawn once, and this page can sit open for hours. What the
   * check must measure against is the window in force when the reader picks,
   * not the one that happened to be current when the field last rendered.
   */
  it("checks a pick against the clock now, not the one the field drew with", () => {
    const onChange = vi.fn();
    render(<StartTimePicker value={null} onChange={onChange} />);
    // Two minutes inside the past allowance when the field was drawn.
    const almostStale = new Date(NOW.getTime() - 24 * 60 * 60 * 1000 + 2 * 60 * 1000);
    // ...and ten minutes of sitting open later, outside it.
    vi.setSystemTime(new Date(NOW.getTime() + 10 * 60 * 1000));

    fireEvent.change(input(), { target: { value: localInputValue(almostStale) } });

    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toHaveTextContent("more than a day in the past");
  });

  it("clears back to nothing chosen", () => {
    const onChange = vi.fn();
    render(<StartTimePicker value={new Date("2026-08-25T07:00")} onChange={onChange} />);

    fireEvent.change(input(), { target: { value: "" } });

    expect(onChange).toHaveBeenCalledWith(null);
  });
});
