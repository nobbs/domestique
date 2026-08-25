import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../storybook/fixtures";
import { MapWidget } from "./MapWidget";

const styleUrl = "https://tiles.openfreemap.org/styles/bright";

const meta = {
  title: "Components/Map/Map Widget",
  component: MapWidget,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <StoryProviders>
        <div className="h-[34rem] overflow-hidden rounded-xl">
          <Story />
        </div>
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof MapWidget>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The reusable base: live cartography and no product-specific layers. */
export const Base: Story = { args: { styleUrl } };
