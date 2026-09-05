import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { RangeSlider } from "./RangeSlider";

const FORMAT = (value: number) => `${value} km`;

describe("RangeSlider", () => {
  it("clamps a range outside its own domain rather than handing it straight to the slider", () => {
    render(
      <RangeSlider
        legend="Distance"
        min={0}
        max={100}
        step={5}
        range={{ min: -20, max: 500 }}
        onChange={vi.fn()}
        format={FORMAT}
        values={[]}
      />,
    );
    expect(screen.getByText("0 km – 100 km")).toBeInTheDocument();
  });

  it("orders the thumbs even when the stored range arrives crossed", () => {
    render(
      <RangeSlider
        legend="Distance"
        min={0}
        max={100}
        step={5}
        range={{ min: 80, max: 20 }}
        onChange={vi.fn()}
        format={FORMAT}
        values={[]}
      />,
    );
    expect(screen.getByText("20 km – 80 km")).toBeInTheDocument();
  });

  it("draws one bar per bin, lit inside the set range and dim outside it", () => {
    const { container } = render(
      <RangeSlider
        legend="Distance"
        min={0}
        max={100}
        step={5}
        range={{ min: 50, max: null }}
        onChange={vi.fn()}
        format={FORMAT}
        values={[10, 60, 60]}
      />,
    );
    const bars = container.querySelectorAll("[aria-hidden] > div");
    expect(bars).toHaveLength(24);
    expect(bars[2]?.className).toContain("bg-[var(--rule)]");
    expect(bars[2]).toHaveStyle({ height: "50%" });
    expect(bars[14]?.className).toContain("bg-[var(--accent)]");
    expect(bars[14]).toHaveStyle({ height: "100%" });
  });
});
