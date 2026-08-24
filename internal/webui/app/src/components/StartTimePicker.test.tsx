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

  it("clears back to nothing chosen", () => {
    const onChange = vi.fn();
    render(<StartTimePicker value={new Date("2026-08-25T07:00")} onChange={onChange} />);

    fireEvent.change(input(), { target: { value: "" } });

    expect(onChange).toHaveBeenCalledWith(null);
  });
});
