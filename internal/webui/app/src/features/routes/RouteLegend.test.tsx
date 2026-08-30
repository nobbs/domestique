import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import type { Position } from "../../api/types";
import type { Highlight } from "../../lib/highlight";
import type { BandShare } from "../../lib/profile";
import { gradientShares } from "../../lib/profile";
import type { SurfaceSummary } from "../../lib/surface";
import { summariseSurface } from "../../lib/surface";
import { RouteLegend } from "./RouteLegend";

function route(pointCount: number): Position[] {
  return Array.from({ length: pointCount }, (_, index) => [8, 49 + index * 0.001] as Position);
}

/** One band and the share of the route it covers, as the page hands them over. */
function band(index: number, share = 0.5): BandShare {
  return { band: index, share };
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

/** The widths of one bar's segments, as percentages, in the order drawn. */
function segmentWidths(label: string): number[] {
  const bar = screen.getByRole("heading", { name: label }).nextElementSibling;
  if (!bar) {
    throw new Error(`expected a bar under ${label}`);
  }

  return [...bar.children].map((segment) =>
    Number.parseFloat((segment as HTMLElement).style.width),
  );
}

describe("RouteLegend", () => {
  it("names every class present and the share it covers", () => {
    render(
      <RouteLegend
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

  /*
   * A row of swatches is only an answer once something says what it is a row of.
   * The two halves say different things about the same route, and a reader who
   * cannot tell which is which has neither.
   */
  it("says which of the two things each half of the key is about", () => {
    render(
      <RouteLegend
        surface={halfGravel()}
        surfaceAbsence="none"
        bands={[band(0)]}
        highlight={null}
        onHighlightChange={() => {}}
      />,
    );

    expect(screen.getByRole("heading", { name: "Gradient" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Surface" })).toBeInTheDocument();
  });

  // The heading is there whether or not there is anything under it to classify:
  // it is what makes the sentence in its place an answer about the surface
  // rather than a stray line in a card.
  it("keeps the surface heading over the reason there is no surface key", () => {
    render(
      <RouteLegend
        surface={null}
        surfaceAbsence="Surface not classified yet."
        bands={[]}
        highlight={null}
        onHighlightChange={() => {}}
      />,
    );

    expect(screen.getByRole("heading", { name: "Surface" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Gradient" })).toBeNull();
  });

  /*
   * Each half is a proportion bar as well as a list of figures: the split is the
   * shape of the ride, and a row of percentages is a table of it. The segments
   * are the shares, so the picture and the figures cannot come to disagree.
   */
  it("draws each mix as a bar in the proportions it just named", () => {
    render(
      <RouteLegend
        surface={halfGravel()}
        surfaceAbsence="none"
        bands={[band(0, 0.25), band(3, 0.75)]}
        highlight={null}
        onHighlightChange={() => {}}
      />,
    );

    expect(segmentWidths("Gradient")).toEqual([25, 75]);
    expect(segmentWidths("Surface")).toEqual([50, 50]);
  });

  it("keeps each gradient segment tied to its named band", () => {
    render(
      <RouteLegend
        surface={halfGravel()}
        surfaceAbsence="none"
        bands={[band(0, 0.25), band(3, 0.75)]}
        highlight={null}
        onHighlightChange={() => {}}
      />,
    );

    const bar = screen.getByRole("heading", { name: "Gradient" }).nextElementSibling;
    expect(
      (bar ? [...bar.children] : []).map((segment) => segment.getAttribute("data-band")),
    ).toEqual(["0", "3"]);
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
      <RouteLegend
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
      <RouteLegend
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
      <RouteLegend
        surface={null}
        surfaceAbsence="Surface not classified yet."
        bands={[band(0), band(2)]}
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
      <RouteLegend
        surface={null}
        surfaceAbsence="none"
        bands={[band(0), band(3)]}
        highlight={null}
        onHighlightChange={() => {}}
      />,
    );

    expect(screen.getByRole("button", { name: /^flat,/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^9%,/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^3%,/ })).toBeNull();
  });

  /*
   * The chips are read as a row of five and are terse about it, so what a chip
   * covers is spoken rather than written: a reader who hears "6%" alone would
   * take it for one gradient rather than for everything from six to nine.
   */
  it("says the span a band covers, and how much of the route it is", () => {
    render(
      <RouteLegend
        surface={null}
        surfaceAbsence="none"
        bands={[band(2, 0.22)]}
        highlight={null}
        onHighlightChange={() => {}}
      />,
    );

    const chip = screen.getByRole("button", { name: /^6%,/ });

    expect(chip).toHaveAccessibleName("6%, 6 to 9%, 22% of the route");
    expect(within(chip).getByText("6%")).toBeInTheDocument();
    expect(within(chip).getByText("22%")).toBeInTheDocument();
  });

  // A gap in the middle of the ramp is the honest answer for a stage that has
  // gentle ground, a drag, and a wall, and nothing in between: the key must not
  // fill it in with a class the chart has nothing to light, and must not stop at
  // it and leave the wall unnamed.
  it("carries a gap in the ramp through from the classification", () => {
    // Eleven metres a point: flat, then a long six percent drag, then fourteen
    // percent — which is bands nought, one, and four, with the others missing.
    const climbing: Position[] = [[8, 49, 100]];
    [...Array(40).fill(0), ...Array(40).fill(6), ...Array(40).fill(14)].forEach(
      (percent, index) => {
        const previous = climbing[index] as [number, number, number];
        climbing.push([8, 49 + (index + 1) * 0.0001, previous[2] + (11.119 * percent) / 100]);
      },
    );
    const bands = gradientShares(climbing);
    expect(bands.map((entry) => entry.band)).toEqual([0, 1, 4]);

    render(
      <RouteLegend
        surface={null}
        surfaceAbsence="none"
        bands={bands}
        highlight={null}
        onHighlightChange={() => {}}
      />,
    );

    expect(screen.getAllByRole("button")).toHaveLength(3);
    expect(screen.getByRole("button", { name: /^flat,/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^3%,/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^12%\+,/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^6%,/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /^9%,/ })).toBeNull();
  });

  it("asks for the class that was clicked", async () => {
    const user = userEvent.setup();
    const onHighlightChange = vi.fn();
    render(
      <RouteLegend
        surface={halfGravel()}
        surfaceAbsence="none"
        bands={[band(2)]}
        highlight={null}
        onHighlightChange={onHighlightChange}
      />,
    );

    await user.click(screen.getByRole("button", { name: /Gravel/ }));
    await user.click(screen.getByRole("button", { name: /^6%,/ }));

    expect(onHighlightChange).toHaveBeenNthCalledWith(1, { type: "surface", kind: "gravel" });
    expect(onHighlightChange).toHaveBeenNthCalledWith(2, { type: "band", band: 2 });
  });

  // A second click on the pressed entry is the way back to the whole route.
  it("gives the whole route back when the pressed class is clicked again", async () => {
    const user = userEvent.setup();
    const onHighlightChange = vi.fn();
    render(
      <RouteLegend
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
      <RouteLegend
        surface={halfGravel()}
        surfaceAbsence="none"
        bands={[band(2)]}
        highlight={null}
        onHighlightChange={() => {}}
      />,
    );
    const steep = screen.getByRole("button", { name: /^6%,/ });
    const asphalt = screen.getByRole("button", { name: /Asphalt/ });
    const gravel = screen.getByRole("button", { name: /Gravel/ });

    await user.tab();

    expect(steep).toHaveFocus();

    // Across the two lists, because the gradient chips are in the same group.
    await user.keyboard("{ArrowRight}");

    expect(asphalt).toHaveFocus();

    await user.keyboard("{ArrowRight}");

    expect(gravel).toHaveFocus();

    // Past the last chip the key is done, rather than trapping the reader.
    await user.tab();

    expect(gravel).not.toHaveFocus();
  });

  it("asks for the class the keyboard is on", async () => {
    const user = userEvent.setup();
    const onHighlightChange = vi.fn();
    render(
      <RouteLegend
        surface={halfGravel()}
        surfaceAbsence="none"
        bands={[band(2)]}
        highlight={null}
        onHighlightChange={onHighlightChange}
      />,
    );

    await user.tab();
    await user.keyboard("{ArrowRight}");
    await user.keyboard("{Enter}");

    expect(onHighlightChange).toHaveBeenCalledWith({ type: "surface", kind: "asphalt" });
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
        <RouteLegend
          surface={halfGravel()}
          surfaceAbsence="none"
          bands={[band(2)]}
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
    const steep = screen.getByRole("button", { name: /^6%,/ });

    expect(gravel).toHaveAttribute("aria-pressed", "true");

    await user.click(steep);

    expect(onHighlightChange).toHaveBeenCalledWith({ type: "band", band: 2 });
    expect(steep).toHaveAttribute("aria-pressed", "true");
    expect(gravel).toHaveAttribute("aria-pressed", "false");

    // And the second press on the band is still the whole route back.
    await user.click(steep);

    expect(onHighlightChange).toHaveBeenLastCalledWith(null);
    expect(steep).toHaveAttribute("aria-pressed", "false");
  });

  it("explains what a class name means, in the name a screen reader hears", () => {
    render(
      <RouteLegend
        surface={halfGravel()}
        surfaceAbsence="none"
        bands={[]}
        highlight={null}
        onHighlightChange={() => {}}
      />,
    );

    expect(screen.getByRole("button", { name: /Gravel/ })).toHaveAccessibleName(
      "Gravel, unpaved and loose, 50% of the route",
    );
  });
});
