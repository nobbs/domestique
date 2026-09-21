import type { Meta, StoryObj } from "@storybook/react-vite";
import { Route, Routes } from "react-router";
import { route, StoryProviders } from "../../storybook/fixtures";
import { AtlasPage } from "./AtlasPage";

// `AtlasPage` renders the real `LibraryMap` — the same live `Source`/`Layer`
// geometry the other content stories keep real — plus a `ScaleControl` that
// assumes a live map context and crashes without one. So this stays live rather
// than joining the chrome stories' deterministic placeholder.
const meta = {
  title: "Features/Atlas/Route Page",
  component: AtlasPage,
  tags: ["autodocs"],
  args: { themeChoice: "system" },
  decorators: [
    (Story) => (
      <StoryProviders path={`/routes/${route.provider}/${route.sourceRouteId}/${route.stageOrder}`}>
        <div className="h-dvh">
          <Routes>
            <Route path="/routes/:provider/:sourceRouteId/:stageOrder" element={<Story />} />
          </Routes>
        </div>
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof AtlasPage>;

export default meta;
type Story = StoryObj<typeof meta>;

/** One route's page as the reader sees it: map, panel, and the dock along the foot. */
export const OpenRoute: Story = {};
