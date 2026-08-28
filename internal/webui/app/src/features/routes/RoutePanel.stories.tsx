import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
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
import { RoutePanel } from "./RoutePanel";
import { RouteProfile } from "./RouteProfile";

// The story holds the state the component reads back, so it renders rather
// than taking args — which is what `component` here would require.
const meta = {
  title: "Features/Atlas/Route Panel",
  tags: ["autodocs"],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

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
              movingSeconds={route.movingSeconds}
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
