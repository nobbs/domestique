/**
 * The climb brackets, against a shown window narrower than the full route.
 */

import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { Climb } from "../../lib/climbs";
import { bandVariable } from "../../lib/mix";
import { gradientBand } from "../../lib/profile";
import { ClimbMarkers } from "./ClimbMarkers";

function climb(startMetres: number, endMetres: number): Climb {
  return {
    startMetres,
    endMetres,
    distanceMetres: endMetres - startMetres,
    ascentMetres: 100,
    averageGradePercent: 8,
    maxGradePercent: 12,
  };
}

/** Three climbs spread across a 10,000 m route. */
const THREE: Climb[] = [climb(0, 1_000), climb(3_000, 4_000), climb(8_000, 9_000)];

describe("ClimbMarkers", () => {
  it("positions every bracket as a fraction of the whole route by default", () => {
    render(<ClimbMarkers climbs={THREE} startMetres={0} endMetres={10_000} onSelect={vi.fn()} />);

    const first = screen.getByRole("button", { name: "Climb 1" });
    expect(first.style.left).toBe("0%");
    expect(first.style.width).toBe("10%");
    const second = screen.getByRole("button", { name: "Climb 2" });
    expect(second.style.left).toBe("30%");
    expect(second.style.width).toBe("10%");
  });

  it("draws no bracket for a climb wholly outside the shown window", () => {
    render(
      <ClimbMarkers climbs={THREE} startMetres={3_000} endMetres={5_000} onSelect={vi.fn()} />,
    );

    expect(screen.queryByRole("button", { name: "Climb 1" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Climb 3" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Climb 2" })).toBeInTheDocument();
  });

  it("clamps a bracket straddling the window's edge instead of overrunning it", () => {
    // Climb 2 runs 3000-4000; the window cuts it off at 3500.
    render(
      <ClimbMarkers climbs={THREE} startMetres={3_500} endMetres={8_500} onSelect={vi.fn()} />,
    );

    const second = screen.getByRole("button", { name: "Climb 2" });
    expect(second.style.left).toBe("0%");
    expect(second.style.width).toBe("10%"); // 500 of the 5000 m window.

    const third = screen.getByRole("button", { name: "Climb 3" });
    expect(third.style.left).toBe("90%");
    expect(third.style.width).toBe("10%"); // clamped to the window's end at 8500.
  });

  it("keeps a climb's full-route ordinal after zooming past an earlier climb", () => {
    render(
      <ClimbMarkers climbs={THREE} startMetres={2_500} endMetres={10_000} onSelect={vi.fn()} />,
    );

    expect(screen.queryByRole("button", { name: "Climb 1" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Climb 2" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Climb 3" })).toBeInTheDocument();
  });

  it("colours a bracket by how steep its climb averages, not one flat colour", () => {
    const climbs: Climb[] = [
      { ...climb(0, 1_000), averageGradePercent: 2 },
      { ...climb(3_000, 4_000), averageGradePercent: 11 },
    ];
    render(<ClimbMarkers climbs={climbs} startMetres={0} endMetres={10_000} onSelect={vi.fn()} />);

    const gentle = screen.getByRole("button", { name: "Climb 1" });
    const steep = screen.getByRole("button", { name: "Climb 2" });

    expect(gentle.style.backgroundColor).toBe(bandVariable(gradientBand(2)));
    expect(steep.style.backgroundColor).toBe(bandVariable(gradientBand(11)));
    expect(gentle.style.backgroundColor).not.toBe(steep.style.backgroundColor);
  });

  it("draws nothing for an empty or inverted window", () => {
    const { container } = render(
      <ClimbMarkers climbs={THREE} startMetres={5_000} endMetres={5_000} onSelect={vi.fn()} />,
    );

    expect(container).toBeEmptyDOMElement();
  });
});
