import { fireEvent, render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { DonutChart } from "./DonutChart";

const SEGMENTS = [
  { key: "a", value: 30, colour: "red" },
  { key: "b", value: 0, colour: "green" },
  { key: "c", value: 70, colour: "blue" },
];

function segment(container: HTMLElement, key: string): SVGPathElement | null {
  return container.querySelector(`path[data-key="${key}"]`);
}

/** The x of the sector's first point, which lies on its outer edge at its start. */
function startX(path: SVGPathElement | null): number {
  const match = path?.getAttribute("d")?.match(/^M ([\d.]+) /);
  return match ? Number(match[1]) : Number.NaN;
}

describe("DonutChart", () => {
  it("draws a sector for each share that has any, in order along the arc", () => {
    const { container } = render(<DonutChart segments={SEGMENTS} />);

    expect(segment(container, "b")).toBeNull();
    expect(segment(container, "a")?.getAttribute("fill")).toBe("red");
    // The arc runs clockwise from the bottom left, so the second share starts
    // further round than the first.
    expect(startX(segment(container, "c"))).toBeGreaterThan(startX(segment(container, "a")));
  });

  it("draws a sliver as a pill rather than nothing", () => {
    const { container } = render(
      <DonutChart
        segments={[
          { key: "a", value: 999, colour: "red" },
          { key: "b", value: 1, colour: "green" },
        ]}
      />,
    );

    expect(segment(container, "b")).not.toBeNull();
  });

  it("names the segment pointed at, and nothing once the pointer leaves it", () => {
    const onActive = vi.fn();
    const { container } = render(<DonutChart segments={SEGMENTS} onActive={onActive} />);

    fireEvent.mouseEnter(segment(container, "c") as Element);
    expect(onActive).toHaveBeenLastCalledWith("c");
    // Into the hole or a gap, which is still inside the ring's box.
    fireEvent.mouseLeave(segment(container, "c") as Element);
    expect(onActive).toHaveBeenLastCalledWith(null);
  });

  it("sets every segment but the named one back", () => {
    const { container } = render(<DonutChart segments={SEGMENTS} active="a" />);

    expect(segment(container, "a")?.getAttribute("opacity")).toBe("1");
    expect(segment(container, "c")?.getAttribute("opacity")).toBe("0.2");
  });

  it("says what it is given beneath the arc", () => {
    const { getByText } = render(<DonutChart segments={SEGMENTS}>2 h</DonutChart>);

    expect(getByText("2 h")).toBeInTheDocument();
  });
});
