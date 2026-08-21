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

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, it } from "vitest";
import type { Position, Route } from "../api/types";
import { routeKey } from "../api/types";
import { RoutePanel } from "../features/routes/RoutePanel";
import { SearchPanel } from "../features/routes/SearchPanel";
import { gradientRanges, presentBands } from "../lib/profile";
import { summariseSurface } from "../lib/surface";
import { expectNoAxeViolations } from "../test/axe";
import { Button } from "./Button";
import { RouteKey } from "./RouteKey";
import { StatusMessage } from "./StatusMessage";

const STAGE: Route = {
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
  it("holds for the search panel, collapsed and grown", async () => {
    for (const query of ["", "alpine"]) {
      const { container, unmount } = render(
        <MemoryRouter>
          <SearchPanel
            shown={[STAGE]}
            total={6}
            query={query}
            onQueryChange={() => {}}
            selectedKey={null}
            onSelect={() => {}}
            onOpen={() => {}}
            shapes={new Map([[routeKey(STAGE), { coordinates: CLIMB }]])}
            readAt="19:38"
          />
        </MemoryRouter>,
      );

      await expectNoAxeViolations(container);
      unmount();
    }
  });

  it("holds for the route card the selected row grew into", async () => {
    const { container } = render(
      <MemoryRouter>
        <SearchPanel
          shown={[STAGE]}
          total={6}
          query=""
          onQueryChange={() => {}}
          selectedKey={routeKey(STAGE)}
          onSelect={() => {}}
          onOpen={() => {}}
          shapes={new Map([[routeKey(STAGE), { coordinates: CLIMB }]])}
          readAt="19:38"
        />
      </MemoryRouter>,
    );

    await expectNoAxeViolations(container);
  });

  it("holds for the route key, picked and unpicked", async () => {
    const bands = presentBands(gradientRanges(CLIMB));
    for (const highlight of [null, { type: "surface", kind: "gravel" } as const]) {
      const { container, unmount } = render(
        <RouteKey
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

  it("holds for the route panel the search swaps to", async () => {
    const { container } = render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter>
          <RoutePanel
            route={STAGE}
            highestMetres={1840}
            subtitle="Alpine loop · read 19:38"
            surface={surfaceSummary()}
            surfaceAbsence="Surface not classified yet."
            bands={presentBands(gradientRanges(CLIMB))}
            highlight={null}
            onHighlightChange={() => {}}
            onClose={() => {}}
            sourceBaseUrl="https://veloplanner.example"
          />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    await expectNoAxeViolations(container);
  });

  it("holds for a status message carrying an action", async () => {
    const { container } = render(
      <StatusMessage title="Nothing here is called that." detail="Search matches route names.">
        <Button variant="standard">Clear search</Button>
      </StatusMessage>,
    );

    await expectNoAxeViolations(container);
  });
});
