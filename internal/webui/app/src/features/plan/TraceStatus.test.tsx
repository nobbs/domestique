import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { TraceSummary } from "./planner";
import { TraceFinished } from "./TraceStatus";

function summary(overrides: Partial<TraceSummary> = {}): TraceSummary {
  return {
    waypoints: { seed: 14, peak: 38, final: 24 },
    rounds: 11,
    seconds: 18,
    followedShare: 0.996,
    strayedStretches: 0,
    planKm: 80.9,
    copiedKm: 81.2,
    outcome: "complete",
    ...overrides,
  };
}

/** Stubs `matchMedia` for one test, restoring it afterwards. */
function stubReducedMotion(reduced: boolean) {
  const restore = window.matchMedia;
  window.matchMedia = (query: string) =>
    ({
      matches: reduced && query.includes("prefers-reduced-motion"),
      media: query,
      onchange: null,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    }) as MediaQueryList;

  return () => {
    window.matchMedia = restore;
  };
}

describe("TraceFinished", () => {
  it("shows no countdown ring for a trace the cap stopped, only a dismiss button", () => {
    render(<TraceFinished summary={summary({ outcome: "stoppedShort" })} onDismiss={() => {}} />);

    expect(screen.getByText("Stopped short · 24 wp")).toBeInTheDocument();
    expect(screen.queryByTestId("trace-countdown")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Dismiss" })).toBeInTheDocument();
  });

  it("shows the remaining seconds as text, not a ring, once motion is reduced", () => {
    const restore = stubReducedMotion(true);
    try {
      render(<TraceFinished summary={summary()} onDismiss={() => {}} />);

      expect(screen.getByText("10s")).toBeInTheDocument();
      expect(screen.queryByTestId("trace-countdown")).not.toBeInTheDocument();
    } finally {
      restore();
    }
  });

  it("draws the countdown as a ring, not text, when motion is not reduced", () => {
    const restore = stubReducedMotion(false);
    try {
      render(<TraceFinished summary={summary()} onDismiss={() => {}} />);

      expect(screen.queryByText("10s")).not.toBeInTheDocument();
      expect(screen.queryByTestId("trace-countdown")).toBeInTheDocument();
    } finally {
      restore();
    }
  });
});
