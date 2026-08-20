import { describe, expect, it } from "vitest";
import { statusQuery } from "./queries";

/**
 * The interval is a function of the last answer, so what it decides is worth
 * asserting directly: a timer that kept asking after a run finished would be
 * asking on nobody's behalf, and one that stopped during a run would leave the
 * operator refreshing the page by hand — the thing the live line exists to
 * spare them.
 */
function intervalWhile(active: unknown): number | false | undefined {
  const { refetchInterval } = statusQuery();
  if (typeof refetchInterval !== "function") {
    throw new Error("the status query no longer decides its own interval");
  }

  return refetchInterval({ state: { data: { sync: { active } } } } as Parameters<
    typeof refetchInterval
  >[0]);
}

describe("statusQuery", () => {
  it("polls while a run has not finished", () => {
    expect(intervalWhile({ targets: 1, stages: { current: 0, pending: 1 } })).toBe(2000);
  });

  it("stops polling once nothing is under way", () => {
    expect(intervalWhile(undefined)).toBe(false);
  });
});
