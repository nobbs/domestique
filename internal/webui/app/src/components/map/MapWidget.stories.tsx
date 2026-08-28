import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../storybook/fixtures";
import { ChromeMap } from "../../storybook/mapMock";
import { MapWidget } from "./MapWidget";

const styleUrl = "https://tiles.openfreemap.org/styles/bright";

const meta = {
  title: "Components/Map/Map Widget",
  component: MapWidget,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <StoryProviders>
        <ChromeMap>
          <div className="h-[34rem] overflow-hidden rounded-xl">
            <Story />
          </div>
        </ChromeMap>
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof MapWidget>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The reusable base: cartography chrome and no product-specific layers. */
export const Base: Story = { args: { styleUrl } };
