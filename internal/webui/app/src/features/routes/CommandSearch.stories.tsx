import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { type Route, routeKey } from "../../api/types";
import { EMPTY_FILTERS, type LibraryFilters, matchesFilters } from "../../lib/filters";
import { matchingRoutes } from "../../lib/library";
import { StoryProviders } from "../../storybook/fixtures";
import { LIBRARY } from "../../storybook/routeLibrary";
import { CommandSearch, type RouteShape } from "./CommandSearch";

const library = LIBRARY.map((entry) => entry.route);
const shapes = new Map<string, RouteShape>(
  LIBRARY.map((entry) => [
    routeKey(entry.route),
    entry.geometry.surface
      ? { coordinates: entry.geometry.coordinates, surface: entry.geometry.surface }
      : { coordinates: entry.geometry.coordinates },
  ]),
);

// The story holds the state the component reads back, so it renders rather
// than taking args — which is what `component` here would require.
const meta = {
  title: "Features/Atlas/Command Search",
  tags: ["autodocs"],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

function Searching({ library }: { library: Route[] }) {
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

export const Panel: Story = { render: () => <Searching library={library} /> };

/** A library from every source, which is when the filters offer a source choice. */
export const MixedSources: Story = {
  render: () => (
    <Searching
      library={library.map((route, index) => ({
        ...route,
        provider: index % 7 === 0 ? "local" : index % 3 === 0 ? "komoot" : "veloplanner",
      }))}
    />
  ),
};
