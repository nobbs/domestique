import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { HistogramChart, histogramBoxes } from "./HistogramChart";

const BARS = [
  { value: 10, colour: "grey", group: 0 },
  { value: 40, colour: "grey", group: 0 },
  { value: 20, colour: "green", group: 1 },
];

function bar(container: HTMLElement, index: number): SVGPathElement | null {
  return container.querySelector(`path[data-bar="${index}"]`);
}

/** The transparent column over one bar, which is what the pointer lands on. */
function column(container: HTMLElement, index: number): Element {
  return container.querySelectorAll("rect:not([data-bar])")[index] as Element;
}

describe("HistogramChart", () => {
  it("draws each bar as tall as its value against the tallest", () => {
    const { container } = render(<HistogramChart label="Spread" bars={BARS} />);
    const boxes = histogramBoxes(BARS, 300);

    expect(screen.getByRole("img", { name: "Spread" })).toBeInTheDocument();
    expect(boxes[1]?.y).toBe(0);
    expect(boxes[0]?.height).toBe((boxes[1]?.height ?? 0) / 4);
    expect(bar(container, 2)?.getAttribute("fill")).toBe("green");
  });

  it("lifts the pointed bar alone, names its group, and reads it out", () => {
    const onActiveGroup = vi.fn();
    const { container } = render(
      <HistogramChart
        label="Spread"
        bars={BARS}
        onActiveGroup={onActiveGroup}
        readout={(index) => `bar ${index}`}
      />,
    );

    fireEvent.mouseEnter(column(container, 1));

    expect(onActiveGroup).toHaveBeenLastCalledWith(0);
    expect(bar(container, 1)?.getAttribute("opacity")).toBe("1");
    // Its group-mate is set back too: pointing at a bar is about that bar.
    expect(bar(container, 0)?.getAttribute("opacity")).toBe("0.2");
    expect(screen.getByText("bar 1")).toBeInTheDocument();

    fireEvent.mouseLeave(container.querySelector("svg") as Element);
    expect(onActiveGroup).toHaveBeenLastCalledWith(null);
    expect(screen.queryByText("bar 1")).not.toBeInTheDocument();
  });

  it("lifts a whole group named from elsewhere", () => {
    const { container } = render(<HistogramChart label="Spread" bars={BARS} activeGroup={0} />);

    expect(bar(container, 0)?.getAttribute("opacity")).toBe("1");
    expect(bar(container, 1)?.getAttribute("opacity")).toBe("1");
    expect(bar(container, 2)?.getAttribute("opacity")).toBe("0.2");
  });

  it("draws a wider bar across more of the axis, as tall as what it holds per unit", () => {
    const boxes = histogramBoxes([{ value: 10 }, { value: 40, span: 4 }], 500);

    // One unit of five is a hundred pixels, less a pixel of ground either side of each bar.
    expect(boxes[0]).toMatchObject({ x: 1, width: 98 });
    expect(boxes[1]).toMatchObject({ x: 101, width: 398 });
    // Forty over four units is ten per unit, the same as the one-unit bar beside it.
    expect(boxes[1]?.height).toBe(boxes[0]?.height);
  });

  it("puts a marker's label where its edge falls", () => {
    render(
      <HistogramChart
        label="Spread"
        bars={[
          { value: 10, colour: "grey", group: 0 },
          { value: 40, span: 4, colour: "grey", group: 0 },
        ]}
        markers={[{ edge: 1, label: "edge" }]}
      />,
    );

    expect(screen.getByText("edge")).toHaveStyle({ left: "20%" });
  });

  it("labels each marker where its edge falls, and the unit at the end", () => {
    render(
      <HistogramChart
        label="Spread"
        bars={BARS}
        markers={[{ edge: 2, label: "120" }]}
        unit="bpm"
      />,
    );

    expect(screen.getByText("120")).toHaveStyle({ left: `${(2 / 3) * 100}%` });
    expect(screen.getByText("bpm")).toBeInTheDocument();
  });
});
