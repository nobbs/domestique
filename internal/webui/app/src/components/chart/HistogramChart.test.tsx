import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { HistogramChart } from "./HistogramChart";

const BARS = [
  { value: 10, colour: "grey", group: 0 },
  { value: 40, colour: "grey", group: 0 },
  { value: 20, colour: "green", group: 1 },
];

function bar(container: HTMLElement, index: number): SVGRectElement | null {
  return container.querySelector(`rect[data-bar="${index}"]`);
}

/** The transparent column over one bar, which is what the pointer lands on. */
function column(container: HTMLElement, index: number): Element {
  return container.querySelectorAll("rect:not([data-bar])")[index] as Element;
}

describe("HistogramChart", () => {
  it("draws each bar as tall as its value against the tallest", () => {
    const { container } = render(<HistogramChart label="Spread" bars={BARS} />);

    expect(screen.getByRole("img", { name: "Spread" })).toBeInTheDocument();
    expect(bar(container, 1)?.getAttribute("height")).toBe("100");
    expect(bar(container, 0)?.getAttribute("height")).toBe("25");
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
