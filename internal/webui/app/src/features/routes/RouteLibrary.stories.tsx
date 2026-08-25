import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { routeKey } from "../../api/types";
import { EMPTY_FILTERS } from "../../lib/filters";
import type { Highlight } from "../../lib/highlight";
import {
  bands,
  climbs,
  coordinates,
  profile,
  route,
  StoryProviders,
  surface,
} from "../../storybook/fixtures";
import { ClimbsList } from "./ClimbsList";
import { FilterPanel } from "./FilterPanel";
import { RoutePanel } from "./RoutePanel";
import { RouteProfile } from "./RouteProfile";
import { SearchPanel } from "./SearchPanel";

const meta = { title: "Features/Route library", tags: ["autodocs"] } satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const FilteringAndResults: Story = {
  render: () => {
    const [query, setQuery] = useState("alpine");
    const [filters, setFilters] = useState(EMPTY_FILTERS);
    const [filtersExpanded, setFiltersExpanded] = useState(false);
    const [selectedKey, setSelectedKey] = useState<string | null>(null);

    return (
      <SearchPanel
        shown={query ? [route] : []}
        total={47}
        query={query}
        onQueryChange={setQuery}
        filters={filters}
        onFiltersChange={setFilters}
        filtersExpanded={filtersExpanded}
        onFiltersExpandedChange={setFiltersExpanded}
        selectedKey={selectedKey}
        onSelect={setSelectedKey}
        onOpen={() => {}}
        shapes={new Map([[routeKey(route), { coordinates }]])}
        readAt="19:38"
        changeOf={() => "new"}
        unitSystem="metric"
      />
    );
  },
};

export const Filters: Story = {
  render: () => {
    const [filters, setFilters] = useState(EMPTY_FILTERS);
    const [expanded, setExpanded] = useState(true);

    return (
      <FilterPanel
        filters={filters}
        onFiltersChange={setFilters}
        expanded={expanded}
        onExpandedChange={setExpanded}
      />
    );
  },
};

export const RouteDetail: Story = {
  render: () => {
    const [highlight, setHighlight] = useState<Highlight | null>(null);
    const [collapsed, setCollapsed] = useState(false);

    return (
      <StoryProviders>
        <RoutePanel
          route={route}
          profile={
            <RouteProfile
              profile={profile}
              title={route.title}
              ascentMetres={route.ascentMetres}
              surface={surface}
              activeMetres={null}
              onActiveChange={() => {}}
              zoomWindow={null}
              onZoomChange={() => {}}
              highlight={highlight}
              collapsed={collapsed}
              onCollapsedChange={setCollapsed}
              unitSystem="metric"
              startAt={null}
              onStartAtChange={() => {}}
              samples={[]}
              coordinates={coordinates}
              rideSeconds={route.movingSeconds}
            />
          }
          highestMetres={295}
          subtitle="Alpine loop · read 19:38"
          surface={surface}
          surfaceAbsence="Surface not classified yet."
          bands={bands}
          highlight={highlight}
          onHighlightChange={setHighlight}
          climbs={climbs}
          onSelectClimb={() => {}}
          libraryCount={47}
          onClose={() => {}}
          sourceBaseUrls={{ veloplanner: "https://veloplanner.com" }}
          unitSystem="metric"
        />
      </StoryProviders>
    );
  },
};

export const Climbs: Story = {
  render: () => <ClimbsList climbs={climbs} onSelect={() => {}} unitSystem="metric" />,
};
