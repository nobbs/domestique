import type { Meta, StoryObj } from "@storybook/react-vite";
import { climbs } from "../../storybook/fixtures";
import { ClimbsList } from "./ClimbsList";

const meta = {
  title: "Features/Atlas/Climbs List",
  component: ClimbsList,
  tags: ["autodocs"],
} satisfies Meta<typeof ClimbsList>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Climbs: Story = {
  args: { climbs, onSelect: () => {}, unitSystem: "metric" },
};
