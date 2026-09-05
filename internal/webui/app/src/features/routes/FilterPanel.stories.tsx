import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { EMPTY_FILTERS } from "../../lib/filters";
import { FilterPanel } from "./FilterPanel";

// The story holds the state the component reads back, so it renders rather
// than taking args — which is what `component` here would require.
const meta = {
  title: "Features/Atlas/Filter Panel",
  tags: ["autodocs"],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const Filters: Story = {
  render: () => {
    const [filters, setFilters] = useState(EMPTY_FILTERS);
    const [expanded, setExpanded] = useState(true);

    return (
      <FilterPanel
        library={[]}
        filters={filters}
        onFiltersChange={setFilters}
        expanded={expanded}
        onExpandedChange={setExpanded}
      />
    );
  },
};
