import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Position } from "../api/types";
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
