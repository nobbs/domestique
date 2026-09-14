import { describe, expect, it } from "vitest";
import { calendarDay, decouplingDomain, definedRuns, median } from "./DecouplingPanel";

describe("decouplingDomain", () => {
  // The median also reads rides from the four weeks before the range, which the dots do not show.
  it("holds the median as well as the rides shown", () => {
    const shown = [{ id: "1", date: "2026-08-23", percent: 4, movingSeconds: 3600 }];

    expect(decouplingDomain(shown, [undefined, 18, 4])).toEqual([-2, 18]);
    expect(decouplingDomain(shown, [-6])).toEqual([-6, 12]);
  });
});

describe("definedRuns", () => {
  it("breaks the line where no median could be taken", () => {
    expect(definedRuns([1, undefined, undefined, 2, 3, undefined])).toEqual([
      [[0, 1]],
      [
        [3, 2],
        [4, 3],
      ],
    ]);
    expect(definedRuns([undefined])).toEqual([]);
  });
});

describe("median", () => {
  it("takes the middle, or the mean of the middle two", () => {
    expect(median([5, 1, 3])).toBe(3);
    expect(median([4, 1, 3, 2])).toBe(2.5);
    expect(median([])).toBeUndefined();
  });
});

describe("calendarDay", () => {
  it("reads the day in the service's zone, not in UTC", () => {
    expect(calendarDay("2026-08-21T22:30:00Z", "Europe/Berlin")).toBe("2026-08-22");
  });
});
