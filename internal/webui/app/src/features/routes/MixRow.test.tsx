import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { MixEntry } from "../../lib/mix";
import { MixRow, placeTags } from "./MixRow";

// Mirrors MixRow.tsx's own constants; keep both in sync if that layout changes.
const TAG_WIDTH = 62;
const TAG_GAP = 6;

function entry(label: string, share: number, metres: number): MixEntry {
  return {
    highlight: { type: "band", band: 0 },
    label,
    description: label,
    share,
    metres,
    colour: "red",
  };
}

describe("placeTags", () => {
  it("keeps a gap between a dominant share and the sliver beside it", () => {
    const places = placeTags([0.64, 0.012, 0.212, 0.093, 0.039], 344);
    for (let index = 1; index < places.length; index++) {
      const previousRight = (places[index - 1]?.left ?? 0) + TAG_WIDTH;
      expect((places[index]?.left ?? 0) - previousRight).toBeGreaterThanOrEqual(TAG_GAP);
    }
  });

  it("keeps the gap after the right-edge fixup pass runs", () => {
    const places = placeTags([0.05, 0.05, 0.05, 0.05, 0.8], 344);
    for (let index = 1; index < places.length; index++) {
      const previousRight = (places[index - 1]?.left ?? 0) + TAG_WIDTH;
      expect((places[index]?.left ?? 0) - previousRight).toBeGreaterThanOrEqual(TAG_GAP - 1e-9);
    }
  });
});

describe("MixRow", () => {
  it("shows a sub-1km class in metres even when its row's longest class is in kilometres", () => {
    render(
      <MixRow
        classesLabel="Surface classes"
        entries={[entry("Asphalt", 0.988, 54_400), entry("Ground", 0.012, 200)]}
        absence="Surface not classified."
        tagSide="below"
        highlight={null}
        onHighlightChange={() => {}}
        unitSystem="metric"
      />,
    );

    expect(screen.getByText("200 m")).toBeInTheDocument();
    expect(screen.getByText("54.4 km")).toBeInTheDocument();
  });

  it("drops the km decimal once the row's longest class passes 100 km", () => {
    render(
      <MixRow
        classesLabel="Gradient bands"
        entries={[entry("flat", 0.9, 120_000), entry("9%", 0.1, 13_000)]}
        absence="No elevation data."
        tagSide="above"
        highlight={null}
        onHighlightChange={() => {}}
        unitSystem="metric"
      />,
    );

    expect(screen.getByText("120 km")).toBeInTheDocument();
    expect(screen.getByText("13 km")).toBeInTheDocument();
  });

  it("renders a faded placeholder bar with sr-only absence copy when there are no classes", () => {
    render(
      <MixRow
        classesLabel="Gradient bands"
        entries={[]}
        absence="No elevation data."
        tagSide="above"
        highlight={null}
        onHighlightChange={() => {}}
        unitSystem="metric"
      />,
    );

    expect(screen.getByText("No elevation data.")).toHaveClass("sr-only");
  });
});
