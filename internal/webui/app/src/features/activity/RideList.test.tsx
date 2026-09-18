/**
 * The ride list, as a reader drives it: weeks newest first, every ride a link
 * to the page that draws it, its weekday and week decided in the service's zone.
 */

import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";
import type { Activity } from "../../api/types";
import { formatAscent, formatDistance, formatDuration, LOCALE } from "../../lib/format";
import { RideList } from "./RideList";

const ZONE = "Europe/Berlin";

/** The day a week's label starts with, in the UI's pinned locale. */
function weekOf(day: number): string {
  return new Intl.DateTimeFormat(LOCALE, { day: "numeric", month: "short", timeZone: ZONE }).format(
    new Date(Date.UTC(2026, 7, day, 12)),
  );
}

function activity(id: number, startedAt: string, overrides: Partial<Activity> = {}): Activity {
  return {
    id: String(id),
    startedAt,
    distanceMetres: 30_000,
    movingSeconds: 3_600,
    elapsedSeconds: 4_000,
    ascentMetres: 300,
    typeId: 40,
    locationId: 0,
    indoor: false,
    provider: "wahoo",
    ...overrides,
  };
}

// Given oldest first, so a list that simply prints what it was handed fails.
const RIDES = [activity(1, "2026-08-19T08:00:00Z"), activity(2, "2026-08-26T08:00:00Z")];

const WEATHER = {
  temperatureMinCelsius: 11.6,
  temperatureMaxCelsius: 18.2,
  windSpeedKmh: 14,
  precipitationMillimetres: 2.4,
  weatherCode: 61,
};

function show(rides: Activity[] = RIDES) {
  render(
    <MemoryRouter>
      <RideList rides={rides} zone={ZONE} />
    </MemoryRouter>,
  );
}

const rideLinks = () =>
  screen
    .getAllByRole("link")
    .filter((link) => link.getAttribute("href")?.startsWith("/activities/"));

describe("the ride list", () => {
  it("lists weeks newest first, each ride linking to its own page", () => {
    show();

    const names = screen.getAllByRole("region").map((region) => region.getAttribute("aria-label"));
    expect(names.findIndex((name) => name?.startsWith(`Week ${weekOf(24)}`))).toBeLessThan(
      names.findIndex((name) => name?.startsWith(`Week ${weekOf(17)}`)),
    );
    expect(rideLinks().map((link) => link.getAttribute("href"))).toEqual([
      "/activities/2",
      "/activities/1",
    ]);
    expect(rideLinks()[0]).toHaveTextContent("30.0 km");
  });

  it("marks a Zwift ride and no other, and names each ride's ground", () => {
    show([
      activity(1, "2026-08-19T08:00:00Z", { provider: "zwift", indoor: true }),
      activity(2, "2026-08-26T08:00:00Z"),
    ]);

    expect(screen.getAllByText("Zwift")).toHaveLength(1);
    expect(screen.getByRole("img", { name: "Indoor" })).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "Outdoor" })).toBeInTheDocument();
  });

  it("shows a ride's max temperature and rain, and no rain for a dry ride", () => {
    show([
      activity(1, "2026-08-19T08:00:00Z", { weather: WEATHER }),
      activity(2, "2026-08-26T08:00:00Z", {
        weather: { ...WEATHER, temperatureMaxCelsius: 15, precipitationMillimetres: 0 },
      }),
      activity(3, "2026-08-27T08:00:00Z"),
    ]);

    expect(screen.getByText("18°")).toBeInTheDocument();
    expect(screen.getByText("15°")).toBeInTheDocument();
    expect(screen.getAllByText(/mm/)).toHaveLength(1);
    expect(screen.getAllByText(/°/)).toHaveLength(2);
  });

  it("dates a ride by its day in the service's own zone, not UTC", () => {
    // 00:30 Monday 24 Aug in Berlin, still Sunday 23 Aug in UTC.
    show([activity(3, "2026-08-23T22:30:00Z")]);

    const week = screen.getByRole("region", { name: new RegExp(`^Week ${weekOf(24)}`) });
    expect(within(week).getByRole("link")).toHaveTextContent("Mon 24 Aug");
  });

  it("adds a week's rides into its heading", () => {
    show([
      activity(1, "2026-08-19T08:00:00Z", { distanceMetres: 40_000, ascentMetres: 300 }),
      activity(2, "2026-08-20T08:00:00Z", {
        distanceMetres: 20_000,
        movingSeconds: 1_800,
        ascentMetres: 150,
      }),
    ]);

    const week = screen.getByRole("region", { name: new RegExp(`^Week ${weekOf(17)}`) });
    expect(
      within(week).getByText(
        `${formatDistance(60_000)} · ${formatDuration(5_400)} · ${formatAscent(450)}`,
      ),
    ).toBeInTheDocument();
  });

  it("draws each ride's bar against the longest ride in the list", () => {
    show([
      activity(1, "2026-08-19T08:00:00Z", { distanceMetres: 60_000 }),
      activity(2, "2026-08-20T08:00:00Z", { distanceMetres: 30_000 }),
    ]);

    const bar = (id: number) =>
      document.querySelector<HTMLElement>(`a[href="/activities/${id}"] > [aria-hidden="true"]`)
        ?.style.width;
    expect(bar(1)).toBe("calc(100% - 0.5rem)");
    expect(bar(2)).toBe("calc(50% - 0.5rem)");
  });

  it("reads start and arrival in the service's zone, arrival after every stop", () => {
    // 08:00Z is 10:00 in Berlin; 4,000 s elapsed lands at 11:06.
    show([activity(1, "2026-08-19T08:00:00Z")]);

    expect(screen.getByRole("link")).toHaveTextContent("10:00–11:06");
  });

  it("groups the same rides by month on request, the month's totals in its heading", async () => {
    show([
      activity(1, "2026-07-30T08:00:00Z"),
      activity(2, "2026-08-19T08:00:00Z"),
      activity(3, "2026-08-26T08:00:00Z"),
    ]);

    await userEvent.click(screen.getByRole("button", { name: "Month" }));

    const months = screen.getAllByRole("region", { name: /^Month / });
    expect(months).toHaveLength(2);
    expect(within(months[0] as HTMLElement).getAllByRole("link")).toHaveLength(2);
    expect(months[0]).toHaveTextContent(formatDistance(60_000));
  });

  it("leaves out a week nobody rode in", () => {
    show();

    expect(screen.getAllByRole("region", { name: /^Week / })).toHaveLength(2);
  });
});
