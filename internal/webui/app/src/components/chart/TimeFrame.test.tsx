import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { formatCalendarDay, frameScale, niceTicks, ReadoutRow, TimeFrame } from "./TimeFrame";

const DATES = ["2026-09-12", "2026-09-13", "2026-09-14", "2026-09-15", "2026-09-16"];

function frame(props: Partial<Parameters<typeof TimeFrame>[0]> = {}) {
  return render(
    <TimeFrame
      label="A test frame"
      dates={DATES}
      panels={[{ height: 50, domain: [0, 10], draw: () => null }]}
      readout={(index) => <ReadoutRow label="value" value={`#${index}`} />}
      {...props}
    />,
  );
}

describe("frameScale", () => {
  it("spreads the days evenly without a projection", () => {
    const scale = frameScale(5);

    expect(scale.at(0)).toBe(0);
    expect(scale.at(2)).toBe(0.5);
    expect(scale.at(4)).toBe(1);
  });

  it("gives the days after today their fixed share, and reads a position back to its day", () => {
    const scale = frameScale(101, 80, 0.25);

    expect(scale.at(80)).toBeCloseTo(0.75);
    expect(scale.at(90)).toBeCloseTo(0.875);
    expect(scale.index(0.875)).toBeCloseTo(90);
    expect(scale.index(0.375)).toBeCloseTo(40);
  });
});

describe("niceTicks", () => {
  it("rules round values across the domain", () => {
    expect(niceTicks(0, 48)).toEqual([0, 20, 40]);
    expect(niceTicks(-55, 40)).toEqual([-50, 0]);
  });
});

describe("TimeFrame", () => {
  it("names itself for a screen reader", () => {
    frame();

    expect(screen.getByRole("img", { name: "A test frame" })).toBeInTheDocument();
  });

  it("walks the crosshair with the arrow keys, from today when one is set", () => {
    frame({ todayIndex: 2 });

    fireEvent.keyDown(screen.getByRole("img").parentElement as HTMLElement, { key: "ArrowRight" });

    expect(screen.getByRole("status")).toHaveTextContent(formatCalendarDay("2026-09-15"));
    expect(screen.getByRole("status")).toHaveTextContent("#3");
  });

  it("only rests on the days it snaps to", () => {
    frame({ snap: [0, 3] });
    const figure = screen.getByRole("img").parentElement as HTMLElement;

    fireEvent.keyDown(figure, { key: "ArrowLeft" });
    expect(screen.getByRole("status")).toHaveTextContent("#0");
    fireEvent.keyDown(figure, { key: "ArrowRight" });
    expect(screen.getByRole("status")).toHaveTextContent("#3");
  });

  // jsdom measures no width, so the frame draws at its narrowest, where every Monday cannot be labelled.
  it("leaves out date labels that would crowd the one before", () => {
    const sixtyDays = Array.from({ length: 60 }, (_, index) =>
      new Date(Date.UTC(2026, 6, 1 + index)).toISOString().slice(0, 10),
    );
    const { container } = frame({ dates: sixtyDays });
    const positions = [...container.querySelectorAll("text")]
      .filter((label) => /^\d+ \w+$/.test(label.textContent ?? ""))
      .map((label) => Number(label.getAttribute("x")));

    expect(positions.length).toBeGreaterThan(1);
    for (const [index, position] of positions.slice(1).entries()) {
      expect(position - (positions[index] ?? 0)).toBeGreaterThanOrEqual(44);
    }
  });

  it("drops the readout when focus leaves", () => {
    frame();
    const figure = screen.getByRole("img").parentElement as HTMLElement;

    fireEvent.keyDown(figure, { key: "ArrowLeft" });
    fireEvent.blur(figure);

    expect(screen.queryByRole("status")).toBeNull();
  });
});
