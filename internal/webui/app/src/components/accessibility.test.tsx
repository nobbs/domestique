/**
 * Every reusable control, put to axe.
 *
 * One file rather than an assertion appended to each component's own suite: the
 * check is the same check everywhere, and a component that arrives without one
 * should fail to be listed here rather than quietly go unexamined. Each entry
 * renders the component the way the application does — a card inside its grid
 * inside a router, a chip inside its group — because most of what axe has to say
 * about a control is about the markup around it.
 *
 * What this cannot see is in `src/test/axe.ts`. The browser suite carries the
 * rest, over the whole page, with the stylesheet loaded.
 */

import { render } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, it } from "vitest";
import type { Position, Stage } from "../api/types";
import { LibraryControls } from "../features/routes/LibraryControls";
import { gradientRanges, presentBands } from "../lib/profile";
import { summariseSurface } from "../lib/surface";
import { expectNoAxeViolations } from "../test/axe";
import { Button } from "./Button";
import { ExploreToggle } from "./ExploreToggle";
import { RouteCard, RouteGrid } from "./RouteCard";
import { StageKey } from "./StageKey";
import { StatusMessage } from "./StatusMessage";

const STAGE: Stage = {
  routeId: 12,
  stageOrder: 2,
  title: "Alpine loop — Descent",
  routeName: "Alpine loop",
  stageName: "Descent",
  sourceRevision: "2026-08-17",
  contentHash: "hash",
  distanceMetres: 42_500,
  ascentMetres: 620,
  maxGradientPercent: 11.4,
  pointCount: 1200,
};

/** A climb steep enough that the key has several bands to offer. */
const CLIMB: Position[] = Array.from(
  { length: 40 },
  (_, index): Position => [8, 49 + index * 0.0004, index * index * 0.6],
);

function surfaceSummary() {
  const summary = summariseSurface(CLIMB, [
    { kind: "asphalt", startIndex: 0, endIndex: 20 },
    { kind: "gravel", startIndex: 21, endIndex: 39 },
  ]);
  if (!summary) {
    throw new Error("expected a summary");
  }

  return summary;
}

describe("accessibility", () => {
  it("holds for a library card in its grid", async () => {
    const { container } = render(
      <MemoryRouter>
        <RouteGrid>
          <li>
            <RouteCard
              stage={STAGE}
              href="/routes/12/2"
              preview={<span aria-hidden="true" />}
              stageCount={3}
            />
          </li>
        </RouteGrid>
      </MemoryRouter>,
    );

    await expectNoAxeViolations(container);
  });

  it("holds for the library controls", async () => {
    const { container } = render(
      <LibraryControls
        query="forest"
        onQueryChange={() => {}}
        sort="name"
        onSortChange={() => {}}
        shown={1}
        total={6}
      />,
    );

    await expectNoAxeViolations(container);
  });

  it("holds for the stage key, picked and unpicked", async () => {
    const bands = presentBands(gradientRanges(CLIMB));
    for (const highlight of [null, { type: "surface", kind: "gravel" } as const]) {
      const { container, unmount } = render(
        <StageKey
          surface={surfaceSummary()}
          surfaceAbsence="Surface not classified yet."
          bands={bands}
          highlight={highlight}
          onHighlightChange={() => {}}
        />,
      );

      await expectNoAxeViolations(container);
      unmount();
    }
  });

  it("holds for the exploration control in both states", async () => {
    for (const exploring of [false, true]) {
      const { container, unmount } = render(
        <ExploreToggle exploring={exploring} onExploringChange={() => {}} />,
      );

      await expectNoAxeViolations(container);
      unmount();
    }
  });

  it("holds for a status message carrying an action", async () => {
    const { container } = render(
      <StatusMessage title="No stages match “forest”." detail="Search matches route names.">
        <Button variant="quiet">Clear search</Button>
      </StatusMessage>,
    );

    await expectNoAxeViolations(container);
  });
});
