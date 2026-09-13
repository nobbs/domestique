import { fireEvent, render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { DonutChart } from "./DonutChart";

const SEGMENTS = [
  { key: "a", value: 30, colour: "red" },
  { key: "b", value: 0, colour: "green" },
  { key: "c", value: 70, colour: "blue" },
];

function segment(container: HTMLElement, key: string): SVGCircleElement | null {
  return container.querySelector(`circle[data-key="${key}"]`);
}

describe("DonutChart", () => {
  it("draws a segment for each share that has any, as a percentage of the ring", () => {
    const { container } = render(<DonutChart segments={SEGMENTS} />);

    expect(segment(container, "b")).toBeNull();
    expect(segment(container, "a")?.getAttribute("stroke-dasharray")).toMatch(/^29\.4 70\.6$/);
    expect(segment(container, "c")?.getAttribute("stroke-dashoffset")).toBe("-30");
  });

  it("names the segment pointed at, and nothing once the pointer leaves", () => {
    const onActive = vi.fn();
    const { container } = render(<DonutChart segments={SEGMENTS} onActive={onActive} />);

    fireEvent.mouseEnter(segment(container, "c") as Element);
    expect(onActive).toHaveBeenLastCalledWith("c");
    fireEvent.mouseLeave(container.querySelector("svg") as Element);
    expect(onActive).toHaveBeenLastCalledWith(null);
  });

  it("sets every segment but the named one back", () => {
    const { container } = render(<DonutChart segments={SEGMENTS} active="a" />);

    expect(segment(container, "a")?.getAttribute("opacity")).toBe("1");
    expect(segment(container, "c")?.getAttribute("opacity")).toBe("0.2");
  });

  it("says what it is given at its centre", () => {
    const { getByText } = render(<DonutChart segments={SEGMENTS}>2 h</DonutChart>);

    expect(getByText("2 h")).toBeInTheDocument();
  });
});
