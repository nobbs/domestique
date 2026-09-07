import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { FitnessDay } from "../../api/types";
import { FitnessChart } from "./FitnessChart";

function day(date: string, overrides: Partial<FitnessDay> = {}): FitnessDay {
  return {
    date,
    trimpLoad: 0,
    trimpFitness: 20,
    trimpFatigue: 25,
    trimpForm: -5,
    tssLoad: 0,
    tssFitness: 40,
    tssFatigue: 50,
    tssForm: -10,
    ...overrides,
  };
}

describe("FitnessChart", () => {
  it("names how many days it covers and which scale it is on", () => {
    render(<FitnessChart days={[day("2026-08-22"), day("2026-08-23")]} scale="tss" />);

    expect(
      screen.getByRole("img", { name: /Fitness, fatigue and form over 2 days/ }),
    ).toBeInTheDocument();
    expect(screen.getByRole("img", { name: /stress score scale/ })).toBeInTheDocument();
  });

  it("reads the other scale when asked for it", () => {
    render(<FitnessChart days={[day("2026-08-22")]} scale="trimp" />);

    expect(screen.getByRole("img", { name: /TRIMP scale/ })).toBeInTheDocument();
  });

  // A day nobody rode has no bar: the gap is the reading.
  it("draws a bar only for a day that carried load", () => {
    const { container } = render(
      <FitnessChart
        days={[
          day("2026-08-22", { tssLoad: 90 }),
          day("2026-08-23"),
          day("2026-08-24", { tssLoad: 40 }),
        ]}
        scale="tss"
      />,
    );

    expect(container.querySelectorAll("rect")).toHaveLength(2);
  });

  // Three series in one frame: fatigue, fitness and form.
  it("draws all three averages", () => {
    const { container } = render(<FitnessChart days={[day("2026-08-22")]} scale="tss" />);

    expect(container.querySelectorAll("polyline")).toHaveLength(3);
  });

  it("draws nothing at all for an empty series", () => {
    const { container } = render(<FitnessChart days={[]} scale="tss" />);

    expect(container).toBeEmptyDOMElement();
  });
});
