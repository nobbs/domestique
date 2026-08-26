import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocationPin } from "./LocationPin";

const meta = {
  title: "Components/Map/Location Pin",
  component: LocationPin,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      // A patch of the accent the map draws over, so the pin's white rim is
      // doing on this page what it does over cartography.
      <div className="grid h-40 w-full place-items-center rounded-xl bg-muted">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof LocationPin>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
