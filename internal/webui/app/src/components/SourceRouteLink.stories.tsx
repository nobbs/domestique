import type { Meta, StoryObj } from "@storybook/react-vite";
import { SourceRouteLink } from "./SourceRouteLink";

const meta = {
  title: "Components/Source Route Link",
  component: SourceRouteLink,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div className="bg-[var(--base)] p-6">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof SourceRouteLink>;

export default meta;
type Story = StoryObj<typeof meta>;

export const VeloPlanner: Story = {
  args: { provider: "veloplanner", baseUrl: "https://veloplanner.com", routeId: 12 },
};
