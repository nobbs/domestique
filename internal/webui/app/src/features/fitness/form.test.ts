import { describe, expect, it } from "vitest";
import {
  bandOf,
  bandTop,
  daysBetween,
  FORM_BANDS,
  formPercent,
  mondayOf,
  RAMP_BANDS,
  reading,
  signed,
  weeklyLoads,
} from "./form";

const DAY = {
  date: "2026-09-14",
  tssLoad: 80,
  tssFitness: 50,
  tssFatigue: 60,
  tssForm: -10,
  trimpLoad: 150,
  trimpFitness: 100,
  trimpFatigue: 130,
  trimpForm: -30,
};

describe("reading", () => {
  it("reads the chosen scale and form as a share of its fitness", () => {
    expect(reading(DAY, "tss")).toMatchObject({ load: 80, fitness: 50, formPercent: -20 });
    expect(reading(DAY, "trimp")).toMatchObject({ load: 150, fitness: 100, formPercent: -30 });
  });

  it("has no form share while there is too little fitness to divide by", () => {
    expect(formPercent(-3, 0.5)).toBe(0);
  });
});

describe("bands", () => {
  it("puts each edge in the band above it", () => {
    expect(bandOf(FORM_BANDS, 20).name).toBe("Transition");
    expect(bandOf(FORM_BANDS, 5).name).toBe("Fresh");
    expect(bandOf(FORM_BANDS, 4.9).name).toBe("Grey zone");
    expect(bandOf(FORM_BANDS, -10).name).toBe("Grey zone");
    expect(bandOf(FORM_BANDS, -30).name).toBe("Optimal");
    expect(bandOf(FORM_BANDS, -30.1).name).toBe("High risk");
  });

  it("reads ramp against its own bands", () => {
    expect(bandOf(RAMP_BANDS, 12).name).toBe("Aggressive");
    expect(bandOf(RAMP_BANDS, 3).name).toBe("Building");
    expect(bandOf(RAMP_BANDS, 0).name).toBe("Holding");
    expect(bandOf(RAMP_BANDS, -4).name).toBe("Detraining");
  });

  it("tops each band at the edge of the one above", () => {
    expect(bandTop(FORM_BANDS, bandOf(FORM_BANDS, 10))).toBe(20);
    expect(bandTop(FORM_BANDS, bandOf(FORM_BANDS, 30))).toBe(Infinity);
  });
});

describe("calendar weeks", () => {
  it("starts a week on its Monday", () => {
    expect(mondayOf("2026-09-14")).toBe("2026-09-14");
    expect(mondayOf("2026-09-20")).toBe("2026-09-14");
    expect(mondayOf("2026-09-01")).toBe("2026-08-31");
  });

  it("counts whole days either way", () => {
    expect(daysBetween("2026-09-14", "2026-09-21")).toBe(7);
    expect(daysBetween("2026-09-14", "2026-09-10")).toBe(-4);
    // Across the clocks going back, which a naive local-time difference would miss.
    expect(daysBetween("2026-10-24", "2026-10-26")).toBe(2);
  });

  it("sums each week's load under its Monday", () => {
    const readings = ["2026-09-13", "2026-09-14", "2026-09-15"].map((date, index) =>
      reading({ ...DAY, date, tssLoad: 10 * (index + 1) }, "tss"),
    );

    expect([...weeklyLoads(readings)]).toEqual([
      ["2026-09-07", 10],
      ["2026-09-14", 50],
    ]);
  });
});

describe("signed", () => {
  it("writes a real minus, and no sign on what rounds to nothing", () => {
    expect(signed(3.4)).toBe("+3");
    expect(signed(-12)).toBe("−12");
    expect(signed(-0.2)).toBe("±0");
    expect(signed(-1.25, 1)).toBe("−1.3");
  });
});
