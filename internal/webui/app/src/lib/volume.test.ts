import { describe, expect, it } from "vitest";
import type { Activity } from "../api/types";
import { bucketActivities, volumeTotals } from "./volume";

/** Local time throughout: the buckets are read in the browser's own zone. */
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
      NOW,
    );

    expect(buckets).toHaveLength(1);
    expect(buckets[0]?.start).toEqual(new Date(2026, 7, 31));
    expect(buckets[0]?.count).toBe(2);
    expect(buckets[0]?.distanceMetres).toBe(60_000);
  });

  it("keeps the weeks nobody rode in, as zeroes", () => {
    const buckets = bucketActivities([activity(new Date(2026, 7, 17, 8))], "week", NOW);

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
      NOW,
    );

    expect(buckets.map((bucket) => bucket.ascentMetres)).toEqual([100, 250]);
    expect(buckets[0]?.label).toContain("2026");
  });

  it("has nothing to bucket without an activity", () => {
    expect(bucketActivities([], "week", NOW)).toEqual([]);
  });

  it("ignores an activity whose start is not a time", () => {
    expect(bucketActivities([activity(NOW, { startedAt: "whenever" })], "week", NOW)).toEqual([]);
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
