import { describe, expect, it } from "vitest";
import { arrivalResolution, forecastResolution } from "./forecastResolution";

describe("forecastResolution", () => {
  it("reads a ride within two days as ICON-D2 at 2 km", () => {
    const resolution = forecastResolution(6);

    expect(resolution.metresPerCell).toBe(2000);
    expect(resolution.sentence).toBe(
      "Within 2 days out, so this uses ICON-D2 — about 2 km resolution.",
    );
  });

  it("keeps ICON-D2 up to and including 48 hours out", () => {
    expect(forecastResolution(48).metresPerCell).toBe(2000);
    expect(forecastResolution(48.1).metresPerCell).toBe(7000);
  });

  it("reads the regional and global band as 7 km, up to 78 hours out", () => {
    expect(forecastResolution(60).sentence).toBe(
      "More than 2 days out: ICON-EU/global guidance, about 7–11 km resolution.",
    );
    expect(forecastResolution(78).metresPerCell).toBe(7000);
  });

  it("reads anything further out as coarser global guidance", () => {
    const resolution = forecastResolution(240);

    expect(resolution.metresPerCell).toBe(11000);
    expect(resolution.sentence).toBe(
      "More than 3 days out: coarser global guidance, past ICON's finer-grained range.",
    );
  });

  it("never sharpens as the lead time grows", () => {
    const cells = [0, 24, 48, 60, 78, 100, 264].map(
      (leadHours) => forecastResolution(leadHours).metresPerCell,
    );

    cells.forEach((cell, index) => {
      expect(cell).toBeGreaterThanOrEqual(cells[index - 1] ?? 0);
    });
  });
});

describe("arrivalResolution", () => {
  it("counts the lead time from now to the first reading of the ride", () => {
    const tomorrow = new Date(Date.now() + 30 * 3_600_000);

    expect(arrivalResolution(tomorrow).metresPerCell).toBe(2000);
  });

  it("reads a ride four days out as the coarser guidance it comes from", () => {
    const later = new Date(Date.now() + 96 * 3_600_000);

    expect(arrivalResolution(later).metresPerCell).toBe(11000);
  });

  it("treats a ride already under way, or one with no reading yet, as sharpest", () => {
    expect(arrivalResolution(new Date(Date.now() - 3_600_000)).metresPerCell).toBe(2000);
    expect(arrivalResolution(undefined).metresPerCell).toBe(2000);
  });
});
