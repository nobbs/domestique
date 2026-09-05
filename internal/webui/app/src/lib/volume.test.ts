import { describe, expect, it } from "vitest";
import type { Activity } from "../api/types";
import { bucketActivities, volumeTotals, windowStart } from "./volume";

// The runner's own zone, so the pre-existing assertions below (none of which
// probe a zone edge) keep reading as local wall-clock times.
const ZONE = Intl.DateTimeFormat().resolvedOptions().timeZone;

function activity(startedAt: Date, overrides: Partial<Activity> = {}): Activity {
  return {
    id: startedAt.getTime(),
    startedAt: startedAt.toISOString(),
    distanceMetres: 30_000,
    movingSeconds: 3_600,
    elapsedSeconds: 4_000,
    ascentMetres: 300,
    typeId: 40,
    locationId: 0,
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
});

describe("windowStart", () => {
  it("is midnight in the given zone, not the runner's own", () => {
    const now = new Date("2026-09-05T10:00:00Z");

    expect(windowStart("Pacific/Auckland", now)).toBe("2025-09-04T12:00:00.000Z");
    expect(windowStart("America/Los_Angeles", now)).toBe("2025-09-05T07:00:00.000Z");
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
