import type { Meta, StoryObj } from "@storybook/react-vite";
import { liveMap, StoryProviders } from "../../storybook/fixtures";
import { AtlasPage } from "./AtlasPage";

// `AtlasPage` renders the real `LibraryMap` — the same route geometry drawn via
// live `Source`/`Layer` that the other "content" stories (`LibraryMap`,
// `RouteOverlay`, ...) already keep real, plus a `ScaleControl` that assumes a
// live map context and crashes without one. So this stays live too, same as
// those, rather than joining the chrome stories' deterministic placeholder.
const meta = {
  title: "Features/Atlas/Entry Page",
  component: AtlasPage,
  tags: ["autodocs"],
  parameters: liveMap,
  args: { themeChoice: "system" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <div className="h-dvh">
          <Story />
        </div>
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof AtlasPage>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The whole entry page as the reader first sees it: map, menu bar, and the browsing panel. */
export const Library: Story = {};
