import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { MixEntry } from "../../lib/mix";
import { MixRow, placeTags } from "./MixRow";

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
});
