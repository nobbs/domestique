import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Position } from "../api/types";
import { summariseSurface } from "../lib/surface";
import { SurfaceBar } from "./SurfaceBar";

function route(pointCount: number): Position[] {
  return Array.from({ length: pointCount }, (_, index) => [8, 49 + index * 0.001] as Position);
}

describe("SurfaceBar", () => {
  it("names every class present and the share it covers", () => {
    const summary = summariseSurface(route(5), [
      { kind: "asphalt", startIndex: 0, endIndex: 1 },
      { kind: "gravel", startIndex: 2, endIndex: 4 },
    ]);
    if (!summary) {
      throw new Error("expected a summary");
    }

    render(<SurfaceBar summary={summary} />);

    expect(screen.getByText("Asphalt")).toBeInTheDocument();
    expect(screen.getByText("Gravel")).toBeInTheDocument();
    expect(screen.getAllByText("50%")).toHaveLength(2);
  });

  it("keeps a sliver of gravel visible in the text rather than rounding it away", () => {
    const summary = summariseSurface(route(201), [
      { kind: "asphalt", startIndex: 0, endIndex: 198 },
      { kind: "gravel", startIndex: 199, endIndex: 200 },
    ]);
    if (!summary) {
      throw new Error("expected a summary");
    }

    render(<SurfaceBar summary={summary} />);

    expect(screen.getByText("<1%")).toBeInTheDocument();
  });

  it("says unsurveyed rather than presenting missing data as a surface", () => {
    const summary = summariseSurface(route(3), [
      { kind: "unknown", startIndex: 0, endIndex: 1 },
      { kind: "asphalt", startIndex: 2, endIndex: 2 },
    ]);
    if (!summary) {
      throw new Error("expected a summary");
    }

    render(<SurfaceBar summary={summary} />);

    expect(screen.getByText("Unsurveyed")).toBeInTheDocument();
  });

  // The strip under the elevation chart carries the proportions in route order;
  // a second bar here would be the same figures twice, one of them unordered.
  it("carries no bar of its own", () => {
    const summary = summariseSurface(route(5), [
      { kind: "asphalt", startIndex: 0, endIndex: 1 },
      { kind: "gravel", startIndex: 2, endIndex: 4 },
    ]);
    if (!summary) {
      throw new Error("expected a summary");
    }

    const { container } = render(<SurfaceBar summary={summary} />);

    expect(container.querySelector(".surface-bar__track")).toBeNull();
  });
});
