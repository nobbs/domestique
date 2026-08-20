import { render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";
import type { Stage } from "../../api/types";
import { StageTable } from "./StageTable";

function stage(routeId: number, stageOrder: number, title: string, distanceMetres: number): Stage {
  return {
    routeId,
    stageOrder,
    routeName: title,
    stageName: "",
    title,
    sourceRevision: "2026-08-17",
    contentHash: `hash-${routeId}-${stageOrder}`,
    distanceMetres,
    ascentMetres: 620,
    maxGradientPercent: 11.4,
    pointCount: 1200,
  };
}

const STAGES = [stage(12, 2, "Alpine loop", 42_500), stage(4, 1, "Valley run", 18_000)];

function renderTable(stages: Stage[] = STAGES) {
  render(
    <MemoryRouter>
      <StageTable stages={stages} />
    </MemoryRouter>,
  );
}

describe("StageTable", () => {
  it("lists the stages it was given, in the order it was given them", () => {
    renderTable();

    const rows = screen.getAllByRole("row").slice(1);
    expect(rows.map((row) => within(row).getByRole("link").textContent)).toEqual([
      "Alpine loop",
      "Valley run",
    ]);
  });

  it("names each column and reads a stage's own name as its row's header", () => {
    renderTable();

    expect(screen.getByRole("columnheader", { name: "Stage" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Distance" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Total climbing" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Steepest gradient" })).toBeInTheDocument();
    expect(screen.getByRole("rowheader", { name: "Alpine loop" })).toBeInTheDocument();
  });

  it("carries each stage's figures in the units the cards use", () => {
    renderTable([STAGES[0] as Stage]);

    const row = screen.getAllByRole("row")[1] as HTMLElement;
    expect(within(row).getByText("42.5 km")).toBeInTheDocument();
    expect(within(row).getByText("620 m")).toBeInTheDocument();
    expect(within(row).getByText("11%")).toBeInTheDocument();
  });

  it("points each row at that stage's own preview", () => {
    renderTable();

    expect(screen.getByRole("link", { name: "Alpine loop" })).toHaveAttribute(
      "href",
      "/routes/12/2",
    );
    expect(screen.getByRole("link", { name: "Valley run" })).toHaveAttribute("href", "/routes/4/1");
  });
});
