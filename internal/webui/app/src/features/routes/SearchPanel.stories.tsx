import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { routeKey } from "../../api/types";
import { EMPTY_FILTERS } from "../../lib/filters";
import { coordinates, route } from "../../storybook/fixtures";
import { SearchPanel } from "./SearchPanel";

// The story holds the state the component reads back, so it renders rather
// than taking args — which is what `component` here would require.
const meta = {
  title: "Features/Atlas/Search Panel",
  tags: ["autodocs"],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

// Enough routes, spread across the measures, for the filter sliders' tracks
// and histograms to show a distribution rather than one bar.
const LIBRARY = Array.from({ length: 47 }, (_, i) => ({
  ...route,
  sourceRouteId: i + 1,
  distanceMetres: 8_000 + ((i * 37) % 23) * 4_000,
  ascentMetres: 50 + ((i * 53) % 19) * 90,
  movingSeconds: 1_500 + ((i * 29) % 17) * 1_100,
}));

export const FilteringAndResults: Story = {
  render: () => {
    const [query, setQuery] = useState("alpine");
    const [filters, setFilters] = useState(EMPTY_FILTERS);
    const [filtersExpanded, setFiltersExpanded] = useState(false);
    const [pickedKey, setPickedKey] = useState<string | null>(null);

    return (
      <SearchPanel
        shown={query ? [route] : []}
        library={LIBRARY}
        query={query}
        onQueryChange={setQuery}
        filters={filters}
        onFiltersChange={setFilters}
        filtersExpanded={filtersExpanded}
        onFiltersExpandedChange={setFiltersExpanded}
        pickedKey={pickedKey}
        onSelect={setPickedKey}
        onOpen={() => {}}
        shapes={new Map([[routeKey(route), { coordinates }]])}
        readAt="19:38"
        changeOf={() => "new"}
      />
    );
  },
};
