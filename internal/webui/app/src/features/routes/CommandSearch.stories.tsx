import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { type Route, routeKey } from "../../api/types";
import { EMPTY_FILTERS, type LibraryFilters, matchesFilters } from "../../lib/filters";
import { matchingRoutes } from "../../lib/library";
import { StoryProviders } from "../../storybook/fixtures";
import { LIBRARY, type SpikeRoute } from "../../storybook/routeLibrary";
import { CommandSearch, type RouteShape } from "./CommandSearch";

function shapesOf(entries: SpikeRoute[]): Map<string, RouteShape> {
  return new Map(
    entries.map((entry) => [
      routeKey(entry.route),
      entry.geometry.surface
        ? { coordinates: entry.geometry.coordinates, surface: entry.geometry.surface }
        : { coordinates: entry.geometry.coordinates },
    ]),
  );
}

const library = LIBRARY.map((entry) => entry.route);
const shapes = shapesOf(LIBRARY);
const MIXED = LIBRARY.map((entry, index) => ({
  ...entry,
  route: {
    ...entry.route,
    provider: index % 7 === 0 ? "local" : index % 3 === 0 ? "komoot" : "veloplanner",
  },
}));

// The story holds the state the component reads back, so it renders rather
// than taking args — which is what `component` here would require.
const meta = {
  title: "Features/Atlas/Command Search",
  tags: ["autodocs"],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

function Searching({ library, shapes }: { library: Route[]; shapes: Map<string, RouteShape> }) {
  const [query, setQuery] = useState("");
  const [filters, setFilters] = useState<LibraryFilters>(EMPTY_FILTERS);
  const [open, setOpen] = useState(true);
  const shown = matchingRoutes(library, query).filter((route) => matchesFilters(route, filters));

  return (
    <StoryProviders>
      <div className="relative h-[640px]">
        <CommandSearch
          open={open}
          onOpenChange={setOpen}
          routeOpen={false}
          library={library}
          shown={shown}
          query={query}
          onQueryChange={setQuery}
          filters={filters}
          onFiltersChange={setFilters}
          activeKey={null}
          onOpen={() => {}}
          shapes={shapes}
          changeOf={() => null}
        />
      </div>
    </StoryProviders>
  );
}

export const Panel: Story = { render: () => <Searching library={library} shapes={shapes} /> };

/** A library from every source, which is when the filters offer a source choice. */
export const MixedSources: Story = {
  render: () => <Searching library={MIXED.map((entry) => entry.route)} shapes={shapesOf(MIXED)} />,
};
