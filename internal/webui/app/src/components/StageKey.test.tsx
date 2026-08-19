import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import type { Position } from "../api/types";
import type { Highlight } from "../lib/highlight";
import { gradientRanges, presentBands } from "../lib/profile";
import type { SurfaceSummary } from "../lib/surface";
import { summariseSurface } from "../lib/surface";
import { StageKey } from "./StageKey";

function route(pointCount: number): Position[] {
  return Array.from({ length: pointCount }, (_, index) => [8, 49 + index * 0.001] as Position);
}

function halfGravel(): SurfaceSummary {
  const summary = summariseSurface(route(5), [
    { kind: "asphalt", startIndex: 0, endIndex: 1 },
    { kind: "gravel", startIndex: 2, endIndex: 4 },
  ]);
  if (!summary) {
    throw new Error("expected a summary");
  }

  return summary;
}

describe("StageKey", () => {
  it("names every class present and the share it covers", () => {
    render(
      <StageKey
        surface={halfGravel()}
        surfaceAbsence="none"
        bands={[]}
        highlight={null}
        onHighlightChange={() => {}}
      />,
    );

    expect(screen.getByText("Asphalt")).toBeInTheDocument();
    expect(screen.getByText("Gravel")).toBeInTheDocument();
    expect(screen.getAllByText("50%")).toHaveLength(2);
  });

  it("keeps a sliver of gravel visible in the text rather than rounding it away", () => {
    const summary = summariseSurface(route(201), [
      { kind: "asphalt", startIndex: 0, endIndex: 198 },
      { kind: "gravel", startIndex: 199, endIndex: 200 },
    ]);
    if (!summary) {
      throw new Error("expected a summary");
    }

    render(
      <StageKey
        surface={summary}
        surfaceAbsence="none"
        bands={[]}
        highlight={null}
        onHighlightChange={() => {}}
      />,
    );

    expect(screen.getByText("<1%")).toBeInTheDocument();
  });

  it("says unsurveyed rather than presenting missing data as a surface", () => {
    const summary = summariseSurface(route(3), [
      { kind: "unknown", startIndex: 0, endIndex: 1 },
      { kind: "asphalt", startIndex: 2, endIndex: 2 },
    ]);
    if (!summary) {
      throw new Error("expected a summary");
    }

    render(
      <StageKey
        surface={summary}
        surfaceAbsence="none"
        bands={[]}
        highlight={null}
        onHighlightChange={() => {}}
      />,
    );

    expect(screen.getByText("Unsurveyed")).toBeInTheDocument();
  });

  it("says why there is no surface key rather than leaving a gap", () => {
    render(
      <StageKey
        surface={null}
        surfaceAbsence="Surface not classified yet."
        bands={[0, 2]}
        highlight={null}
        onHighlightChange={() => {}}
      />,
    );

    expect(screen.getByText("Surface not classified yet.")).toBeInTheDocument();
  });

  // Offering a band the stage does not have would be offering a selection that
  // lights nothing.
  it("lists only the bands it was told the stage has", () => {
    render(
      <StageKey
        surface={null}
        surfaceAbsence="none"
        bands={[0, 3]}
        highlight={null}
        onHighlightChange={() => {}}
      />,
    );

    expect(screen.getByRole("button", { name: "< 4%" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "12–16%" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "4–8%" })).toBeNull();
  });

  // A gap in the middle of the ramp is the honest answer for a stage that has
  // gentle ground, a drag, and a wall, and nothing in between: the key must not
  // fill it in with a class the chart has nothing to light, and must not stop at
  // it and leave the wall unnamed.
  it("carries a gap in the ramp through from the classification", () => {
    // Eleven metres a point: flat, then a long six percent drag, then fourteen
    // percent — which is bands nought, one, and three, with two missing.
    const climbing: Position[] = [[8, 49, 100]];
    [...Array(40).fill(0), ...Array(40).fill(6), ...Array(40).fill(14)].forEach(
      (percent, index) => {
        const previous = climbing[index] as [number, number, number];
        climbing.push([8, 49 + (index + 1) * 0.0001, previous[2] + (11.119 * percent) / 100]);
      },
    );
    const bands = presentBands(gradientRanges(climbing));
    expect(bands).toEqual([0, 1, 3]);

    render(
      <StageKey
        surface={null}
        surfaceAbsence="none"
        bands={bands}
        highlight={null}
        onHighlightChange={() => {}}
      />,
    );

    expect(screen.getAllByRole("button")).toHaveLength(3);
    expect(screen.getByRole("button", { name: "< 4%" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "4–8%" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "12–16%" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "8–12%" })).toBeNull();
    expect(screen.queryByRole("button", { name: "≥ 16%" })).toBeNull();
  });

  it("asks for the class that was clicked", async () => {
    const user = userEvent.setup();
    const onHighlightChange = vi.fn();
    render(
      <StageKey
        surface={halfGravel()}
        surfaceAbsence="none"
        bands={[2]}
        highlight={null}
        onHighlightChange={onHighlightChange}
      />,
    );

    await user.click(screen.getByRole("button", { name: /Gravel/ }));
    await user.click(screen.getByRole("button", { name: "8–12%" }));

    expect(onHighlightChange).toHaveBeenNthCalledWith(1, { type: "surface", kind: "gravel" });
    expect(onHighlightChange).toHaveBeenNthCalledWith(2, { type: "band", band: 2 });
  });

  // A second click on the pressed entry is the way back to the whole route.
  it("gives the whole route back when the pressed class is clicked again", async () => {
    const user = userEvent.setup();
    const onHighlightChange = vi.fn();
    render(
      <StageKey
        surface={halfGravel()}
        surfaceAbsence="none"
        bands={[]}
        highlight={{ type: "surface", kind: "gravel" }}
        onHighlightChange={onHighlightChange}
      />,
    );
    const gravel = screen.getByRole("button", { name: /Gravel/ });

    expect(gravel).toHaveAttribute("aria-pressed", "true");
    await user.click(gravel);

    expect(onHighlightChange).toHaveBeenCalledWith(null);
  });

  // Both lists are chips of one group, so the group is one tab stop and the
  // arrows move within it: eleven classes cost one Tab to reach, not eleven.
  it("holds the whole key on one tab stop and moves inside it with the arrows", async () => {
    const user = userEvent.setup();
    render(
      <StageKey
        surface={halfGravel()}
        surfaceAbsence="none"
        bands={[2]}
        highlight={null}
        onHighlightChange={() => {}}
      />,
    );
    const asphalt = screen.getByRole("button", { name: /Asphalt/ });
    const gravel = screen.getByRole("button", { name: /Gravel/ });
    const band = screen.getByRole("button", { name: "8–12%" });

    await user.tab();

    expect(asphalt).toHaveFocus();

    await user.keyboard("{ArrowRight}");

    expect(gravel).toHaveFocus();

    // Across the two lists, because the gradient chips are in the same group.
    await user.keyboard("{ArrowRight}");

    expect(band).toHaveFocus();

    // Past the last chip the key is done, rather than trapping the reader.
    await user.tab();

    expect(band).not.toHaveFocus();
  });

  it("asks for the class the keyboard is on", async () => {
    const user = userEvent.setup();
    const onHighlightChange = vi.fn();
    render(
      <StageKey
        surface={halfGravel()}
        surfaceAbsence="none"
        bands={[2]}
        highlight={null}
        onHighlightChange={onHighlightChange}
      />,
    );

    await user.tab();
    await user.keyboard("{ArrowRight}");
    await user.keyboard("{Enter}");

    expect(onHighlightChange).toHaveBeenCalledWith({ type: "surface", kind: "gravel" });
  });

  // One selection over both lists: a gradient band replaces a pressed surface
  // rather than adding to it, which is the question the highlight can answer.
  // Held above the key, as the page holds it, so the pressed chips are the ones
  // the answer produced rather than the ones the key decided on its own.
  it("gives up the pressed surface when a gradient band is picked", async () => {
    const user = userEvent.setup();
    const onHighlightChange = vi.fn();

    function Held() {
      const [highlight, setHighlight] = useState<Highlight | null>({
        type: "surface",
        kind: "gravel",
      });

      return (
        <StageKey
          surface={halfGravel()}
          surfaceAbsence="none"
          bands={[2]}
          highlight={highlight}
          onHighlightChange={(next) => {
            onHighlightChange(next);
            setHighlight(next);
          }}
        />
      );
    }

    render(<Held />);
    const gravel = screen.getByRole("button", { name: /Gravel/ });
    const band = screen.getByRole("button", { name: "8–12%" });

    expect(gravel).toHaveAttribute("aria-pressed", "true");

    await user.click(band);

    expect(onHighlightChange).toHaveBeenCalledWith({ type: "band", band: 2 });
    expect(band).toHaveAttribute("aria-pressed", "true");
    expect(gravel).toHaveAttribute("aria-pressed", "false");

    // And the second press on the band is still the whole route back.
    await user.click(band);

    expect(onHighlightChange).toHaveBeenLastCalledWith(null);
    expect(band).toHaveAttribute("aria-pressed", "false");
  });

  it("explains what a class name means, in the name a screen reader hears", () => {
    render(
      <StageKey
        surface={halfGravel()}
        surfaceAbsence="none"
        bands={[]}
        highlight={null}
        onHighlightChange={() => {}}
      />,
    );

    expect(screen.getByRole("button", { name: /Gravel/ })).toHaveAccessibleName(
      "Gravel, unpaved and loose, 50% of the stage",
    );
  });
});
