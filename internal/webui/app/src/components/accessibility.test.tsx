/**
 * Every reusable control with no Storybook story of its own, put to axe.
 *
 * Every component with a story gets this same check automatically from the
 * Storybook suite's `a11y` parameter — see `.storybook/preview.tsx` — over
 * every state a story demonstrates, not only the one rendered here. What
 * remains in this file is what has no story to carry that check yet: a
 * component that gains one should have its entry here removed rather than
 * kept alongside it, so a control is examined here exactly once.
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
import { FilterPanel } from "../features/routes/FilterPanel";
import { RoutePanel } from "../features/routes/RoutePanel";
import { RouteProfile } from "../features/routes/RouteProfile";
import { SearchPanel } from "../features/routes/SearchPanel";
import { findClimbs } from "../lib/climbs";
import { EMPTY_FILTERS } from "../lib/filters";
import { buildProfile, gradientShares } from "../lib/profile";
import { summariseSurface } from "../lib/surface";
import { expectNoAxeViolations } from "../test/axe";

const STAGE: Route = {
  provider: "veloplanner",
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
            filters={EMPTY_FILTERS}
            onFiltersChange={() => {}}
            filtersExpanded={false}
            onFiltersExpandedChange={() => {}}
            selectedKey={null}
            onSelect={() => {}}
            onOpen={() => {}}
            shapes={new Map([[routeKey(STAGE), { coordinates: CLIMB }]])}
            readAt="19:38"
            changeOf={() => "new"}
            unitSystem="metric"
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
          filters={EMPTY_FILTERS}
          onFiltersChange={() => {}}
          filtersExpanded={false}
          onFiltersExpandedChange={() => {}}
          selectedKey={routeKey(STAGE)}
          onSelect={() => {}}
          onOpen={() => {}}
          shapes={new Map([[routeKey(STAGE), { coordinates: CLIMB }]])}
          readAt="19:38"
          changeOf={() => "updated"}
          unitSystem="metric"
        />
      </MemoryRouter>,
    );

    await expectNoAxeViolations(container);
  });

  it("holds for the library filters, expanded with an active filter", async () => {
    const { container } = render(
      <MemoryRouter>
        <SearchPanel
          shown={[STAGE]}
          total={6}
          query=""
          onQueryChange={() => {}}
          filters={{ ...EMPTY_FILTERS, surfaces: ["gravel"] }}
          onFiltersChange={() => {}}
          filtersExpanded={true}
          onFiltersExpandedChange={() => {}}
          selectedKey={null}
          onSelect={() => {}}
          onOpen={() => {}}
          shapes={new Map([[routeKey(STAGE), { coordinates: CLIMB }]])}
          readAt="19:38"
          changeOf={() => null}
          unitSystem="metric"
        />
      </MemoryRouter>,
    );

    await expectNoAxeViolations(container);
  });

  it("holds for the route panel the search swaps to", async () => {
    const { container } = render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter>
          <RoutePanel
            route={STAGE}
            profile={
              <RouteProfile
                profile={buildProfile(CLIMB)}
                title={STAGE.title}
                ascentMetres={STAGE.ascentMetres}
                surface={surfaceSummary()}
                activeMetres={null}
                onActiveChange={() => {}}
                zoomWindow={null}
                onZoomChange={() => {}}
                highlight={null}
                collapsed={false}
                onCollapsedChange={() => {}}
                unitSystem="metric"
                startAt={null}
                onStartAtChange={() => {}}
                samples={[]}
                coordinates={CLIMB}
              />
            }
            highestMetres={1840}
            subtitle="Alpine loop · read 19:38"
            surface={surfaceSummary()}
            surfaceAbsence="Surface not classified yet."
            bands={gradientShares(CLIMB)}
            highlight={null}
            onHighlightChange={() => {}}
            climbs={findClimbs(CLIMB)}
            onSelectClimb={() => {}}
            libraryCount={47}
            onClose={() => {}}
            sourceBaseUrls={{ veloplanner: "https://veloplanner.example" }}
            unitSystem="metric"
          />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    await expectNoAxeViolations(container);
  });

  /*
   * Unfolded as well as folded, because what unfolds is a dialog: the trigger
   * alone passing says nothing about the popup the names are actually in.
   */
  it("holds for the library filters, folded and unfolded", async () => {
    for (const expanded of [true, false]) {
      const { container, unmount } = render(
        <FilterPanel
          filters={EMPTY_FILTERS}
          onFiltersChange={() => {}}
          expanded={expanded}
          onExpandedChange={() => {}}
        />,
      );

      await expectNoAxeViolations(expanded ? document.body : container);
      unmount();
    }
  });
});
