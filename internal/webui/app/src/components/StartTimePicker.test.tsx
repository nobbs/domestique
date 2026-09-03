import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StartTimePicker } from "./StartTimePicker";

const NOW = new Date("2026-08-24T12:00:00Z");

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  vi.setSystemTime(NOW);
});

afterEach(() => {
  vi.useRealTimers();
});

/** The half of the control that sets the time of day. */
function timeField(): HTMLInputElement {
  return screen.getByLabelText("The time the ride starts") as HTMLInputElement;
}

/** The half that opens the calendar, which also reports the day chosen. */
function dayButton(): HTMLElement {
  return screen.getByLabelText("The day the ride starts");
}

describe("StartTimePicker", () => {
  it("opens empty when nothing has been chosen", () => {
    render(<StartTimePicker value={null} onChange={() => {}} />);

    expect(dayButton()).toHaveTextContent("Pick a day");
    expect(timeField().value).toBe("");
  });

  it("treats clearing the time field as nothing, not as midnight", () => {
    const onChange = vi.fn();
    render(<StartTimePicker value={new Date("2026-08-25T07:00")} onChange={onChange} />);

    // An empty string splits to [""] and Number("") is 0 — without the guard
    // this proposed a confident midnight the reader never picked.
    fireEvent.change(timeField(), { target: { value: "" } });

    expect(onChange).not.toHaveBeenCalled();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("keeps the time field inert until a day exists to set it on", () => {
    render(<StartTimePicker value={null} onChange={() => {}} />);

    expect(timeField()).toBeDisabled();
  });

  it("holds a picked day until a time joins it", async () => {
    const onChange = vi.fn();
    render(<StartTimePicker value={null} onChange={onChange} />);

    fireEvent.click(dayButton());
    // The morning after "now", as the calendar offers it.
    fireEvent.click(await screen.findByRole("button", { name: /25(th)?/ }));

    // A day alone is not a departure: nothing proposed, nothing refused, and
    // the time field now has a day to set its hours on.
    expect(onChange).not.toHaveBeenCalled();
    expect(timeField()).toBeEnabled();
    expect(dayButton()).not.toHaveTextContent("Pick a day");

    fireEvent.change(timeField(), { target: { value: "08:30" } });

    expect(onChange).toHaveBeenCalledWith(new Date("2026-08-25T08:30"));
  });

  it("accepts a time inside the forecast window", () => {
    const onChange = vi.fn();
    // The morning after "now": comfortably inside both bounds of the window.
    render(<StartTimePicker value={new Date("2026-08-25T07:00")} onChange={onChange} />);

    fireEvent.change(timeField(), { target: { value: "08:30" } });

    expect(onChange).toHaveBeenCalledWith(new Date("2026-08-25T08:30"));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("refuses a time more than a day in the past, and says so", () => {
    const onChange = vi.fn();
    render(<StartTimePicker value={new Date("2026-08-01T07:00")} onChange={onChange} />);

    fireEvent.change(timeField(), { target: { value: "08:00" } });

    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByText("That's more than a day in the past.")).toBeInTheDocument();
  });

  it("refuses a time beyond the forecast horizon, and says so", () => {
    const onChange = vi.fn();
    render(<StartTimePicker value={new Date("2026-09-20T07:00")} onChange={onChange} />);

    fireEvent.change(timeField(), { target: { value: "08:00" } });

    expect(onChange).not.toHaveBeenCalled();
    expect(
      screen.getByText("That ride would finish past the 16-day forecast horizon."),
    ).toBeInTheDocument();
  });

  it("refuses a start whose finish would fall past the horizon", () => {
    const onChange = vi.fn();
    // A start the horizon would allow on its own, for a ride long enough that
    // its finish would not be.
    render(
      <StartTimePicker
        value={new Date("2026-09-09T07:00")}
        onChange={onChange}
        movingSeconds={20 * 3600}
      />,
    );

    fireEvent.change(timeField(), { target: { value: "20:00" } });

    expect(onChange).not.toHaveBeenCalled();
    expect(
      screen.getByText("That ride would finish past the 16-day forecast horizon."),
    ).toBeInTheDocument();
  });

  it("offers a later start for a short ride than for a long one", () => {
    const onChange = vi.fn();
    const late = new Date("2026-09-08T09:00");

    const short = render(<StartTimePicker value={late} onChange={onChange} movingSeconds={600} />);
    fireEvent.change(timeField(), { target: { value: "10:00" } });
    expect(onChange).toHaveBeenCalledTimes(1);
    short.unmount();

    onChange.mockClear();
    render(<StartTimePicker value={late} onChange={onChange} movingSeconds={40 * 3600} />);
    fireEvent.change(timeField(), { target: { value: "10:00" } });

    expect(onChange).not.toHaveBeenCalled();
  });

  it("checks a pick against the clock now, not the one the field drew with", () => {
    const onChange = vi.fn();
    render(<StartTimePicker value={new Date("2026-08-25T07:00")} onChange={onChange} />);

    // The page has been open for a fortnight; the window has moved on without
    // this render being told.
    vi.setSystemTime(new Date("2026-09-08T12:00:00Z"));
    fireEvent.change(timeField(), { target: { value: "08:00" } });

    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByText("That's more than a day in the past.")).toBeInTheDocument();
  });

  /*
   * WebKit reports a time field's segments only once it loses focus: a
   * departure typed into it and left focused reached nothing, and the page
   * showed a day, a time and no forecast. Written as the browser reports it —
   * the field carries the value, and blur is the first event about it.
   */
  it("commits a time the browser only reports on blur", async () => {
    const onChange = vi.fn();
    render(<StartTimePicker value={null} onChange={onChange} />);

    fireEvent.click(dayButton());
    fireEvent.click(await screen.findByRole("button", { name: /25(th)?/ }));

    const field = timeField();
    field.value = "08:30";
    fireEvent.blur(field);

    expect(onChange).toHaveBeenCalledWith(new Date("2026-08-25T08:30"));
  });

  it("leaves an unfilled field alone when it loses focus", async () => {
    const onChange = vi.fn();
    render(<StartTimePicker value={null} onChange={onChange} />);

    // A day first: an empty field is disabled until one exists, and blurring
    // a disabled control is not a state a reader can reach.
    fireEvent.click(dayButton());
    fireEvent.click(await screen.findByRole("button", { name: /25(th)?/ }));

    fireEvent.blur(timeField());

    expect(onChange).not.toHaveBeenCalled();
  });

  it("proposes nothing further when a field it already accepted loses focus", () => {
    const onChange = vi.fn();
    render(<StartTimePicker value={new Date("2026-08-25T07:00")} onChange={onChange} />);

    // What a browser firing both events looks like from here.
    fireEvent.change(timeField(), { target: { value: "08:30" } });
    fireEvent.blur(timeField());

    expect(onChange).toHaveBeenCalledTimes(1);
  });

  it("says which half of the departure is still missing", async () => {
    render(<StartTimePicker value={null} onChange={() => {}} />);

    expect(screen.queryByText("Add a time to forecast this ride.")).not.toBeInTheDocument();

    fireEvent.click(dayButton());
    fireEvent.click(await screen.findByRole("button", { name: /25(th)?/ }));

    // A day alone draws the same button a chosen departure does, so without
    // this the reader is left with a picker that looks finished and a forecast
    // that never appears.
    expect(screen.getByText("Add a time to forecast this ride.")).toBeInTheDocument();
  });

  it("clears back to nothing chosen", () => {
    const onChange = vi.fn();
    render(<StartTimePicker value={new Date("2026-08-25T07:00")} onChange={onChange} />);

    fireEvent.click(screen.getByLabelText("Clear the ride start"));

    expect(onChange).toHaveBeenCalledWith(null);
  });
});
