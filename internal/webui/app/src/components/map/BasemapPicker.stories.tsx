import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { BasemapPicker } from "./BasemapPicker";

const streets = { name: "Streets", styleUrl: "", darkCartography: false };
const basemaps = [streets, { name: "Dark", styleUrl: "", darkCartography: true }];

function Picker() {
  const [selectedName, setSelectedName] = useState(streets.name);
  const [expanded, setExpanded] = useState(true);

  return (
    <BasemapPicker
      basemaps={basemaps}
      selectedName={selectedName}
      onSelect={setSelectedName}
      expanded={expanded}
      onExpandedChange={setExpanded}
    />
  );
}

const meta = {
  title: "Components/Map/Basemap Picker",
  component: BasemapPicker,
  tags: ["autodocs"],
  args: {
    basemaps,
    selectedName: streets.name,
    onSelect: () => {},
    expanded: true,
    onExpandedChange: () => {},
  },
  decorators: [
    (Story) => (
      <div className="w-64 bg-[var(--base)] p-6">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof BasemapPicker>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Picker /> };
