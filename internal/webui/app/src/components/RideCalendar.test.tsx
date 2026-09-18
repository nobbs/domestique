import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Activity } from "../api/types";
import { RideCalendar, riddenDays } from "./RideCalendar";

function ride(startedAt: string, indoor: boolean): Activity {
  return {
    id: startedAt,
    startedAt,
    distanceMetres: 1,
    movingSeconds: 1,
    elapsedSeconds: 1,
    ascentMetres: 0,
    typeId: 0,
    locationId: 0,
    indoor,
    provider: "wahoo",
  };
}

describe("riddenDays", () => {
  it("reads the day in the service's zone and marks a day ridden on both grounds as both", () => {
    const days = riddenDays(
      [
        // 00:30 on Monday 24 Aug in Berlin, still Sunday in UTC.
        ride("2026-08-23T22:30:00Z", true),
        ride("2026-08-24T17:00:00Z", false),
        ride("2026-08-25T17:00:00Z", true),
        ride("not a time", false),
      ],
      "Europe/Berlin",
    );

    expect([...days.entries()]).toEqual([
      ["2026-08-24", "both"],
      ["2026-08-25", "indoor"],
    ]);
  });
});

describe("RideCalendar", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("returns to this month from its icon, wherever the reader has paged", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(new Date("2026-09-17T12:00:00Z"));
    render(<RideCalendar rides={[ride("2026-06-10T08:00:00Z", false)]} zone="Europe/Berlin" />);

    expect(screen.getByRole("heading", { name: "June 2026" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Previous month" }));
    await userEvent.click(screen.getByRole("button", { name: "This month" }));

    expect(screen.getByRole("heading", { name: "September 2026" })).toBeInTheDocument();
  });

  it("fades the days still to come and goes no further than this month", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(new Date("2026-09-17T12:00:00Z"));
    render(<RideCalendar rides={[ride("2026-08-10T08:00:00Z", false)]} zone="Europe/Berlin" />);

    const next = screen.getByRole("button", { name: "Next month" });
    expect(next).toBeEnabled();
    await userEvent.click(next);

    expect(screen.getByRole("heading", { name: "September 2026" })).toBeInTheDocument();
    expect(next).toBeDisabled();
    expect(screen.getByRole("img", { name: "17 September 2026: no ride" })).not.toHaveClass(
      "opacity-35",
    );
    expect(screen.getByRole("img", { name: "18 September 2026: still to come" })).toHaveClass(
      "opacity-35",
    );
  });
});
