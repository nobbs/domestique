import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";
import type { Stage } from "../api/types";
import { StageList } from "./StageList";

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

function renderList(stages: Stage[], at = "/") {
  return render(
    <MemoryRouter initialEntries={[at]}>
      <StageList stages={stages} hrefFor={(s) => `/routes/${s.routeId}/${s.stageOrder}`} />
    </MemoryRouter>,
  );
}

describe("StageList", () => {
  it("links each stage to its map view", () => {
    renderList([stage()]);

    const link = screen.getByRole("link", { name: /Alpine loop/ });
    expect(link).toHaveAttribute("href", "/routes/12/1");
  });

  it("shows the stored distance and point count", () => {
    renderList([stage()]);

    expect(screen.getByText(/42\.5 km/)).toBeInTheDocument();
    expect(screen.getByText(/1,200 points/)).toBeInTheDocument();
  });

  it("marks the stage matching the current address as active", () => {
    renderList([stage(), stage({ routeId: 13, stageOrder: 2, title: "Sunday" })], "/routes/13/2");

    const active = screen.getByRole("link", { name: /Sunday/ });
    expect(active.className).toContain("stage-list__item--active");
  });

  it("explains an empty library instead of rendering nothing", () => {
    renderList([]);

    expect(screen.getByText(/No stages have been synced yet/)).toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });
});
