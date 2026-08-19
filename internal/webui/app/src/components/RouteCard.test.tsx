import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";
import type { Stage } from "../api/types";
import { RouteCard, RouteGrid } from "./RouteCard";

function stage(overrides: Partial<Stage> = {}): Stage {
  return {
    routeId: 12,
    stageOrder: 1,
    title: "Alpine loop — Descent",
    routeName: "Alpine loop",
    stageName: "Descent",
    sourceRevision: "2026-08-17",
    contentHash: "hash",
    distanceMetres: 42_500,
    ascentMetres: 620,
    maxGradientPercent: 11.4,
    pointCount: 1200,
    ...overrides,
  };
}

function renderCard(value: Stage = stage(), stageCount?: number) {
  return render(
    <MemoryRouter>
      <RouteGrid>
        <li>
          <RouteCard
            stage={value}
            href={`/routes/${value.routeId}/${value.stageOrder}`}
            preview={<span data-testid="preview" />}
            stageCount={stageCount}
          />
        </li>
      </RouteGrid>
    </MemoryRouter>,
  );
}

describe("RouteCard", () => {
  it("links the whole card to the route preview", () => {
    renderCard();

    expect(screen.getByRole("link", { name: /Alpine loop/ })).toHaveAttribute(
      "href",
      "/routes/12/1",
    );
  });

  it("shows distance, climbing, and the steepest gradient", () => {
    renderCard();

    expect(screen.getByText(/42\.5 km/)).toBeInTheDocument();
    expect(screen.getByText(/620 m/)).toBeInTheDocument();
    expect(screen.getByText(/11%/)).toBeInTheDocument();
  });

  it("does not claim statistics a route has no profile for", () => {
    renderCard(stage({ ascentMetres: 0, maxGradientPercent: 0 }));

    expect(screen.getByTitle("Total climbing")).toHaveTextContent("—");
    expect(screen.getByTitle("Steepest sustained gradient")).toHaveTextContent("—");
  });

  it("names in words what each figure's symbol stands for", () => {
    renderCard();

    // The symbols carried their meaning in a tooltip, which a pointer can reach
    // and a screen reader or a touch device cannot. Asserted through the link's
    // accessible name so the words have to be in the document, not just in an
    // attribute. The name is computed by flattening the card, which trims each
    // element's own text, so the gap between a label and its figure is optional.
    expect(screen.getByRole("link", { name: /Total climbing\s*620 m/ })).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /Steepest sustained gradient\s*11%/ }),
    ).toBeInTheDocument();
  });

  it("renders whatever preview it is given", () => {
    renderCard();

    expect(screen.getByTestId("preview")).toBeInTheDocument();
  });

  it("says which stage of its route a split route's card is", () => {
    renderCard(stage({ stageOrder: 2 }), 3);

    expect(screen.getByText("Stage 2 of 3")).toBeInTheDocument();
    // The route it belongs to stays named on the card, so the position has
    // something to be a position in.
    expect(screen.getByRole("link", { name: /Alpine loop/ })).toBeInTheDocument();
  });

  it("says nothing about stages for a route that has only one", () => {
    renderCard(stage({ stageName: "", title: "Alpine loop" }), 1);

    expect(screen.queryByText(/^Stage /)).not.toBeInTheDocument();
  });

  it("says nothing about stages when the count is not known", () => {
    renderCard();

    expect(screen.queryByText(/^Stage /)).not.toBeInTheDocument();
  });

  it("renders a very long name in full rather than cutting it short", () => {
    const long = `${"Grand Traverse of the Upper Rhine Valley ".repeat(4)}Stage one`;
    renderCard(stage({ title: long, routeName: long, stageName: "" }));

    expect(screen.getByRole("link", { name: new RegExp(long) })).toBeInTheDocument();
  });

  it("still renders a stage whose geometry has not been cached yet", () => {
    renderCard(
      stage({
        distanceMetres: 0,
        ascentMetres: 0,
        maxGradientPercent: 0,
        pointCount: 0,
        title: "Not yet synced",
      }),
    );

    expect(screen.getByRole("link", { name: /Not yet synced/ })).toBeInTheDocument();
    expect(screen.getByTitle("Total climbing")).toHaveTextContent("—");
  });
});
