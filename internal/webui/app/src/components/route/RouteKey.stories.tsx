import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { Highlight } from "../../lib/highlight";
import { bands, surface } from "../../storybook/fixtures";
import { RouteKey } from "./RouteKey";

function Key() {
  const [highlight, setHighlight] = useState<Highlight | null>(null);

  return (
    <RouteKey
      surface={surface}
      surfaceAbsence="Surface not classified yet."
      bands={bands}
      highlight={highlight}
      onHighlightChange={setHighlight}
    />
  );
}

const meta = {
  title: "Components/Route/Route Key",
  component: RouteKey,
  tags: ["autodocs"],
  args: {
    surface: null,
    surfaceAbsence: "",
    bands: [],
    highlight: null,
    onHighlightChange: () => {},
  },
  decorators: [
    (Story) => (
      <div className="max-w-md bg-[var(--base)] p-6">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof RouteKey>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Key /> };
