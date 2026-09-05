/**
 * The ribbon and the class names beneath it, without a route behind them.
 */

import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { groundSegments } from "../../lib/mix";
import type { SurfaceSummary } from "../../lib/surface";
import { GroundRibbon } from "./GroundRibbon";

/** Asphalt for most of the route, with one gravel stretch worth naming. */
function surface(): SurfaceSummary {
  const bands = [
    { kind: "asphalt" as const, startMetres: 0, endMetres: 8_000 },
    { kind: "gravel" as const, startMetres: 8_000, endMetres: 10_000 },
  ];

  return {
    bands,
    shares: [
      { kind: "asphalt", metres: 8_000, share: 0.8 },
      { kind: "gravel", metres: 2_000, share: 0.2 },
    ],
    totalMetres: 10_000,
  };
}

function show(options: { thin?: boolean; unmarked?: ["asphalt"] | [] } = {}) {
  const s = surface();
  return render(
    <GroundRibbon
      segments={groundSegments(s)}
      surface={s}
      labelled
      thin={options.thin ?? false}
      unmarked={options.unmarked ?? []}
      highlight={null}
      onHighlightChange={vi.fn()}
    />,
  );
}

describe("GroundRibbon", () => {
  it("draws the bar at the ordinary height by default", () => {
    const { container } = show();

    expect(container.querySelector(".h-3")).toBeInTheDocument();
    expect(container.querySelector(".h-1\\.5")).not.toBeInTheDocument();
  });

  it("draws the bar thin when asked", () => {
    const { container } = show({ thin: true });

    expect(container.querySelector(".h-1\\.5")).toBeInTheDocument();
    expect(container.querySelector(".h-3")).not.toBeInTheDocument();
  });

  it("gives an unmarked class no toggle button and draws its segment transparent", () => {
    const { container } = show({ unmarked: ["asphalt"] });

    expect(screen.queryByRole("button", { name: /Asphalt/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Gravel/ })).toBeInTheDocument();
    const asphaltSegment = [...container.querySelectorAll<HTMLElement>(".h-3 > div")][0];
    expect(asphaltSegment?.style.background).toBe("transparent");
  });

  it("keeps an unmarked segment's width, so the marked segments stay in place", () => {
    const withoutUnmarking = show();
    const marked = show({ unmarked: ["asphalt"] });

    const widthOf = (view: ReturnType<typeof show>) =>
      [...view.container.querySelectorAll<HTMLElement>(".h-3 > div")].map(
        (segment) => segment.style.flexGrow,
      );

    expect(widthOf(marked)).toEqual(widthOf(withoutUnmarking));
  });

  it("still labels a marked class once", () => {
    show();

    expect(screen.getAllByRole("button", { name: /Gravel/ })).toHaveLength(1);
    expect(screen.getAllByRole("button", { name: /Asphalt/ })).toHaveLength(1);
  });
});
