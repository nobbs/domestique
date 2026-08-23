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
import { RouteProfile } from "../features/routes/RouteProfile";
import { SearchPanel } from "../features/routes/SearchPanel";
import { findClimbs } from "../lib/climbs";
import { EMPTY_FILTERS } from "../lib/filters";
import { buildProfile, gradientShares } from "../lib/profile";
import { summariseSurface } from "../lib/surface";
import { expectNoAxeViolations } from "../test/axe";
import { BasemapPicker } from "./BasemapPicker";
import { Button } from "./Button";
import { MapCredits } from "./MapCredits";
import { RouteKey } from "./RouteKey";
import { StatusMessage } from "./StatusMessage";
import { ThemePicker } from "./ThemePicker";

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
        />
      </MemoryRouter>,
    );

    await expectNoAxeViolations(container);
  });

  it("holds for the route key, picked and unpicked", async () => {
    const bands = gradientShares(CLIMB);
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
          />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    await expectNoAxeViolations(container);
  });

  it("holds for the map credit, folded and unfolded", async () => {
    // Both halves, because the folded one is a button standing on its own with
    // the text it names no longer in the document.
    for (const expanded of [true, false]) {
      const { container, unmount } = render(
        <QueryClientProvider client={new QueryClient()}>
          <MapCredits
            styleUrl={undefined}
            extra="Surface data © OpenStreetMap contributors"
            choice={expanded}
            onChoiceChange={() => {}}
          />
        </QueryClientProvider>,
      );

      await expectNoAxeViolations(container);
      unmount();
    }
  });

  it("holds for the basemap chooser, folded and unfolded", async () => {
    // Both halves, for the same reason the credit is checked both ways: folded
    // it is a button on its own, and the group it names is not in the document.
    for (const expanded of [true, false]) {
      const { container, unmount } = render(
        <BasemapPicker
          basemaps={[
            {
              name: "Streets",
              styleUrl: "https://tiles.example.test/bright",
              darkCartography: false,
            },
            {
              name: "Satellite",
              styleUrl: "https://imagery.example.test/hybrid",
              darkCartography: true,
            },
          ]}
          selectedName="Streets"
          onSelect={() => {}}
          expanded={expanded}
          onExpandedChange={() => {}}
        />,
      );

      await expectNoAxeViolations(container);
      unmount();
    }
  });

  it("holds for the theme chooser, folded and unfolded", async () => {
    for (const expanded of [true, false]) {
      const { container, unmount } = render(
        <ThemePicker
          choice="system"
          onChoose={() => {}}
          expanded={expanded}
          onExpandedChange={() => {}}
        />,
      );

      await expectNoAxeViolations(container);
      unmount();
    }
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
