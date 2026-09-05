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

export const FilteringAndResults: Story = {
  render: () => {
    const [query, setQuery] = useState("alpine");
    const [filters, setFilters] = useState(EMPTY_FILTERS);
    const [filtersExpanded, setFiltersExpanded] = useState(false);
    const [pickedKey, setPickedKey] = useState<string | null>(null);

    return (
      <SearchPanel
        shown={query ? [route] : []}
        library={[route]}
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
