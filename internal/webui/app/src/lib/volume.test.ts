import { describe, expect, it } from "vitest";
import type { Activity } from "../api/types";
import {
  bucketActivities,
  volumeTotals,
  weekdayIndex,
  weekRangeLabel,
  weeksWithRides,
} from "./volume";

// The runner's own zone, so the pre-existing assertions below (none of which
// probe a zone edge) keep reading as local wall-clock times.
const ZONE = Intl.DateTimeFormat().resolvedOptions().timeZone;

function activity(startedAt: Date, overrides: Partial<Activity> = {}): Activity {
  return {
    id: String(startedAt.getTime()),
    startedAt: startedAt.toISOString(),
    distanceMetres: 30_000,
    movingSeconds: 3_600,
    elapsedSeconds: 4_000,
    ascentMetres: 300,
    typeId: 40,
    locationId: 0,
    provider: "wahoo",
    ...overrides,
  };
}

const NOW = new Date(2026, 8, 5, 12); // Saturday 5 September 2026

describe("bucketActivities", () => {
  it("starts a week on the Monday and runs newest first", () => {
    const buckets = bucketActivities(
      [activity(new Date(2026, 8, 5, 8)), activity(new Date(2026, 7, 31, 8))],
      "week",
      ZONE,
      NOW,
    );

    expect(buckets).toHaveLength(1);
    expect(buckets[0]?.start).toEqual(new Date(2026, 7, 31));
    expect(buckets[0]?.count).toBe(2);
    expect(buckets[0]?.distanceMetres).toBe(60_000);
  });

  it("keeps the weeks nobody rode in, as zeroes", () => {
    const buckets = bucketActivities([activity(new Date(2026, 7, 17, 8))], "week", ZONE, NOW);

    expect(buckets.map((bucket) => bucket.count)).toEqual([0, 0, 1]);
    expect(buckets.map((bucket) => bucket.start)).toEqual([
      new Date(2026, 7, 31),
      new Date(2026, 7, 24),
      new Date(2026, 7, 17),
    ]);
  });

  it("gathers a month from its first day", () => {
    const buckets = bucketActivities(
      [
        activity(new Date(2026, 8, 1, 8), { ascentMetres: 100 }),
        activity(new Date(2026, 7, 31, 8), { ascentMetres: 250 }),
      ],
      "month",
      ZONE,
      NOW,
    );

    expect(buckets.map((bucket) => bucket.ascentMetres)).toEqual([100, 250]);
    expect(buckets[0]?.label).toContain("2026");
  });

  it("has nothing to bucket without an activity", () => {
    expect(bucketActivities([], "week", ZONE, NOW)).toEqual([]);
  });

  it("ignores an activity whose start is not a time", () => {
    expect(bucketActivities([activity(NOW, { startedAt: "whenever" })], "week", ZONE, NOW)).toEqual(
      [],
    );
  });

  // Buckets run newest first back to the ride's own bucket; a `now` weeks
  // after the ride keeps the assertion about that one bucket, not position 0.
  const LATER = new Date("2026-10-15T12:00:00Z");

  it("buckets a ride already into Monday in one zone but still Sunday in another", () => {
    // 2026-09-06T13:00:00Z is 01:00 Monday in Auckland but only 06:00 Sunday in
    // Los Angeles: the same instant, two different weeks.
    const rideInstant = new Date("2026-09-06T13:00:00Z");
    const auckland = bucketActivities([activity(rideInstant)], "week", "Pacific/Auckland", LATER);
    const losAngeles = bucketActivities(
      [activity(rideInstant)],
      "week",
      "America/Los_Angeles",
      LATER,
    );

    expect(auckland.find((bucket) => bucket.count > 0)?.start.toISOString()).toBe(
      "2026-09-06T12:00:00.000Z",
    );
    expect(losAngeles.find((bucket) => bucket.count > 0)?.start.toISOString()).toBe(
      "2026-08-31T07:00:00.000Z",
    );
  });

  it("moves a month edge with the zone the same way", () => {
    // 2026-09-01T05:00:00Z is already 1 September in Auckland but still 31
    // August in Los Angeles.
    const rideInstant = new Date("2026-09-01T05:00:00Z");
    const auckland = bucketActivities([activity(rideInstant)], "month", "Pacific/Auckland", LATER);
    const losAngeles = bucketActivities(
      [activity(rideInstant)],
      "month",
      "America/Los_Angeles",
      LATER,
    );

    expect(auckland.find((bucket) => bucket.count > 0)?.start.toISOString()).toBe(
      "2026-08-31T12:00:00.000Z",
    );
    expect(losAngeles.find((bucket) => bucket.count > 0)?.start.toISOString()).toBe(
      "2026-08-01T07:00:00.000Z",
    );
  });

  it("spans multiple years back to the earliest activity", () => {
    const buckets = bucketActivities(
      [activity(new Date(2023, 8, 5, 8)), activity(new Date(2026, 8, 1, 8))],
      "month",
      ZONE,
      NOW,
    );

    expect(buckets).toHaveLength(37);
    expect(buckets[0]?.count).toBe(1);
    expect(buckets.at(-1)?.count).toBe(1);
    expect(buckets.slice(1, -1).every((bucket) => bucket.count === 0)).toBe(true);
  });
});

describe("volumeTotals", () => {
  it("adds every activity up", () => {
    const totals = volumeTotals([
      activity(new Date(2026, 8, 5, 8)),
      activity(new Date(2026, 6, 5, 8), { distanceMetres: 10_000, movingSeconds: 1_200 }),
    ]);

    expect(totals).toEqual({
      distanceMetres: 40_000,
      movingSeconds: 4_800,
      ascentMetres: 600,
      count: 2,
    });
  });
});

it("leaves an activity with an unreadable start out of the totals too", () => {
  const broken = activity(new Date(2026, 7, 20, 10), { startedAt: "not a date" });
  const totals = volumeTotals([activity(new Date(2026, 7, 20, 10)), broken]);

  expect(totals.count).toBe(1);
  expect(totals.distanceMetres).toBe(30_000);
});

describe("weeksWithRides", () => {
  it("attaches each activity to its own week, in the order given", () => {
    const first = activity(new Date(2026, 8, 5, 8));
    const second = activity(new Date(2026, 8, 5, 18));
    const weeks = weeksWithRides([first, second], ZONE, NOW);

    expect(weeks).toHaveLength(1);
    expect(weeks[0]?.rides).toEqual([first, second]);
  });

  it("leaves a rideless week with an empty rides array", () => {
    const weeks = weeksWithRides([activity(new Date(2026, 7, 17, 8))], ZONE, NOW);

    expect(weeks.map((week) => week.rides.length)).toEqual([0, 0, 1]);
  });
});

describe("weekdayIndex", () => {
  it("reads Monday as 0 in the given zone", () => {
    expect(weekdayIndex(activity(new Date("2026-08-24T10:00:00Z")), "UTC")).toBe(0);
  });

  it("follows the zone across a UTC day boundary", () => {
    // 2026-08-23T22:30:00Z is 00:30 Monday 24 Aug in Europe/Berlin but is
    // still Sunday 23 Aug in UTC — the same instant, two different weekdays.
    const ride = activity(new Date("2026-08-23T22:30:00Z"));

    expect(weekdayIndex(ride, "UTC")).toBe(6);
    expect(weekdayIndex(ride, "Europe/Berlin")).toBe(0);
  });
});

describe("weekRangeLabel", () => {
  // Through the platform's own formatter, so the assertion carries no locale
  // of its own and still fails if the range ends on the wrong day.
  it("spans the Monday to the Sunday six days later", () => {
    const day = (at: Date) =>
      new Intl.DateTimeFormat(undefined, {
        day: "numeric",
        month: "short",
        timeZone: "UTC",
      }).format(at);

    expect(weekRangeLabel(new Date(Date.UTC(2026, 7, 31)), "UTC")).toBe(
      `${day(new Date(Date.UTC(2026, 7, 31)))} – ${day(new Date(Date.UTC(2026, 8, 6)))}`,
    );
  });
});
