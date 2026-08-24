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
    render(<StartTimePicker value={null} onChange={onChange} rideSeconds={sixHours} />);

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
      <StartTimePicker value={null} onChange={() => {}} rideSeconds={60 * 60} />,
    );
    const shortRideMax = input().getAttribute("max") ?? "";
    unmount();

    render(<StartTimePicker value={null} onChange={() => {}} rideSeconds={10 * 60 * 60} />);

    expect(input().getAttribute("max") ?? "").not.toBe("");
    expect((input().getAttribute("max") ?? "") < shortRideMax).toBe(true);
  });

  it("clears back to nothing chosen", () => {
    const onChange = vi.fn();
    render(<StartTimePicker value={new Date("2026-08-25T07:00")} onChange={onChange} />);

    fireEvent.change(input(), { target: { value: "" } });

    expect(onChange).toHaveBeenCalledWith(null);
  });
});
