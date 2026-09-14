import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { FitnessScaleOutlook } from "../../api/types";
import { reading } from "./form";
import { SeasonChart } from "./SeasonChart";

const day = (date: string, load: number) =>
  reading(
    {
      date,
      tssLoad: load,
      tssFitness: 40,
      tssFatigue: 45,
      tssForm: -5,
      trimpLoad: 0,
      trimpFitness: 0,
      trimpFatigue: 0,
      trimpForm: 0,
    },
    "tss",
  );

const projected = Array.from({ length: 21 }, (_, index) => ({
  date: new Date(Date.UTC(2026, 8, 15 + index)).toISOString().slice(0, 10),
  fitness: 40,
  fatigue: 40,
  form: 0,
}));

const OUTLOOK: FitnessScaleOutlook = {
  rampPerWeek: 1,
  habitualDailyLoad: 40,
  weekLoadLow: 300,
  weekLoadHigh: 360,
  plans: [
    { plan: "rest", dailyLoad: 0, days: projected },
    { plan: "habitual", dailyLoad: 40, days: projected },
    { plan: "build", dailyLoad: 48, days: projected },
  ],
};

describe("SeasonChart", () => {
  // The last day served is a Monday, so its week has begun and nothing past today has been ridden.
  it("keeps every week's load bar left of the today line", () => {
    const readings = [
      day("2026-09-10", 80),
      day("2026-09-11", 0),
      day("2026-09-12", 120),
      day("2026-09-13", 60),
      day("2026-09-14", 90),
    ];
    const { container } = render(
      <SeasonChart readings={readings} outlook={OUTLOOK} scaleName="stress score" />,
    );

    const today = container.querySelector('line[stroke-dasharray="1 3"]');
    const bars = [...container.querySelectorAll('rect[fill="var(--ink-2)"]')];
    expect(today).not.toBeNull();
    expect(bars).toHaveLength(2);
    const todayX = Number(today?.getAttribute("x1"));
    for (const bar of bars) {
      expect(Number(bar.getAttribute("x")) + Number(bar.getAttribute("width"))).toBeLessThanOrEqual(
        todayX,
      );
    }
  });
});
