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
    pointCount: 1200,
    ...overrides,
  };
}

function renderCard(value: Stage = stage()) {
  return render(
    <MemoryRouter>
      <RouteGrid>
        <li>
          <RouteCard
            stage={value}
            href={`/routes/${value.routeId}/${value.stageOrder}`}
            preview={<span data-testid="preview" />}
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

  it("shows the stored distance and point count", () => {
    renderCard();

    expect(screen.getByText(/42\.5 km/)).toBeInTheDocument();
    expect(screen.getByText(/1,200 points/)).toBeInTheDocument();
  });

  it("renders whatever preview it is given", () => {
    renderCard();

    expect(screen.getByTestId("preview")).toBeInTheDocument();
  });

  it("still renders a stage whose geometry has not been cached yet", () => {
    renderCard(stage({ distanceMetres: 0, pointCount: 0, title: "Not yet synced" }));

    expect(screen.getByRole("link", { name: /Not yet synced/ })).toBeInTheDocument();
    expect(screen.getByText(/—/)).toBeInTheDocument();
  });
});
