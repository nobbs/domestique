import type { Meta, StoryObj } from "@storybook/react-vite";
import { MixBar } from "./MixBar";

const meta = {
  title: "Components/Route/Mix Bar",
  component: MixBar,
  tags: ["autodocs"],
  args: {
    children: (
      <>
        <span className="block min-w-px bg-[var(--surface-asphalt)]" style={{ width: "60%" }} />
        <span className="block min-w-px bg-[var(--surface-gravel)]" style={{ width: "30%" }} />
        <span className="block min-w-px bg-[var(--surface-unsurveyed)]" style={{ width: "10%" }} />
      </>
    ),
  },
  decorators: [
    (Story) => (
      <div className="w-64 bg-[var(--panel)] p-4">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof MixBar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Segments: Story = {};
