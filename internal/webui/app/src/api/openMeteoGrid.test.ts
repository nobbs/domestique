import { describe, expect, it } from "vitest";
import { gridWindow, omUrl } from "./openMeteoGrid";

describe("omUrl", () => {
  it("names the run's hour directory and drops the colon and Z from the stamp", () => {
    expect(omUrl(new Date("2026-09-05T12:00:00Z"), "2026-09-05T15:00Z")).toBe(
      "https://openmeteo.s3.amazonaws.com/data_spatial/dwd_icon_d2/2026/09/05/1200Z/2026-09-05T1500.om",
    );
  });

  it("pads a single-digit month, day and hour", () => {
    expect(omUrl(new Date("2026-01-02T03:00:00Z"), "2026-01-02T03:00Z")).toBe(
      "https://openmeteo.s3.amazonaws.com/data_spatial/dwd_icon_d2/2026/01/02/0300Z/2026-01-02T0300.om",
    );
  });
});

describe("gridWindow", () => {
  it("floors the low edge and ceils the high edge to the file's 32-cell chunks", () => {
    expect(gridWindow([7.0, 48.0, 8.0, 49.0])).toEqual({ x0: 544, x1: 608, y0: 224, y1: 320 });
  });

  it("clamps a bbox that overruns the model's own domain to it", () => {
    expect(gridWindow([-100, -100, 100, 100])).toEqual({ x0: 0, x1: 1215, y0: 0, y1: 746 });
  });

  it("collapses to an empty window for a bbox entirely outside the domain", () => {
    expect(gridWindow([50, 60, 51, 61])).toEqual({ x0: 1215, x1: 1215, y0: 746, y1: 746 });
  });
});
