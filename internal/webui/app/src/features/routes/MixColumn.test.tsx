/**
 * The upright bar, checked for what it says rather than how it looks.
 *
 * Two things here are load-bearing and neither is visual: that a class is
 * labelled with the ground it covers rather than its share of the route, and
 * that a plain click always reports its own class, pressed or not — stepping
 * is the highlight owner's business, not this bar's — while alt-click reports
 * the whole route back.
 */

import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { surfaceEntries } from "../../lib/mix";
import type { SurfaceSummary } from "../../lib/surface";
import { MixColumn } from "./MixColumn";

function ground(): SurfaceSummary {
  return {
    bands: [
      { kind: "asphalt", startMetres: 0, endMetres: 80_000 },
      { kind: "gravel", startMetres: 80_000, endMetres: 100_000 },
    ],
    shares: [
      { kind: "asphalt", metres: 80_000, share: 0.8 },
      { kind: "gravel", metres: 20_000, share: 0.2 },
    ],
    totalMetres: 100_000,
  };
}

function renderColumn(onHighlightChange = vi.fn(), highlight = null) {
  render(
    <MixColumn
      name="Surface"
      classesLabel="Surface classes"
      entries={surfaceEntries(ground())}
      absence="Surface not classified yet."
      highlight={highlight}
      onHighlightChange={onHighlightChange}
    />,
  );

  return onHighlightChange;
}

describe("MixColumn", () => {
  it("labels each class with the ground it covers, not its share", () => {
    renderColumn();

    expect(screen.getByText("80.0 km")).toBeInTheDocument();
    expect(screen.getByText("20.0 km")).toBeInTheDocument();
    expect(screen.queryByText("80%")).toBeNull();
  });

  it("still speaks both quantities, whichever one is drawn", () => {
    renderColumn();

    expect(
      screen.getByRole("button", { name: /Gravel,.*20\.0 km, 20% of the route/ }),
    ).toBeInTheDocument();
  });

  it("says nothing at all for a route nobody has classified", () => {
    render(
      <MixColumn
        name="Surface"
        classesLabel="Surface classes"
        entries={[]}
        absence="Surface not classified yet."
        highlight={null}
        onHighlightChange={() => {}}
      />,
    );

    expect(screen.getByText("Surface not classified yet.")).toBeInTheDocument();
  });

  it("picks a class out, and reports it again on a second plain press", async () => {
    const onHighlightChange = vi.fn();
    const { rerender } = render(
      <MixColumn
        name="Surface"
        classesLabel="Surface classes"
        entries={surfaceEntries(ground())}
        absence="Surface not classified yet."
        highlight={null}
        onHighlightChange={onHighlightChange}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: /^Gravel,/ }));
    expect(onHighlightChange).toHaveBeenLastCalledWith({ type: "surface", kind: "gravel" });

    rerender(
      <MixColumn
        name="Surface"
        classesLabel="Surface classes"
        entries={surfaceEntries(ground())}
        absence="Surface not classified yet."
        highlight={{ type: "surface", kind: "gravel" }}
        onHighlightChange={onHighlightChange}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: /^Gravel,/ }));

    expect(onHighlightChange).toHaveBeenLastCalledWith({ type: "surface", kind: "gravel" });
  });

  it("gives the whole route back on an alt-click", async () => {
    const onHighlightChange = vi.fn();
    render(
      <MixColumn
        name="Surface"
        classesLabel="Surface classes"
        entries={surfaceEntries(ground())}
        absence="Surface not classified yet."
        highlight={{ type: "surface", kind: "gravel" }}
        onHighlightChange={onHighlightChange}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /^Gravel,/ }), { altKey: true });

    expect(onHighlightChange).toHaveBeenLastCalledWith(null);
  });
});
