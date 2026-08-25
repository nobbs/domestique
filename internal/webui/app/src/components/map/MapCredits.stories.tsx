import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { StoryProviders } from "../../storybook/fixtures";
import { MapCredits } from "./MapCredits";

function Credits() {
  const [choice, setChoice] = useState<boolean | null>(null);

  return (
    <MapCredits
      styleUrl={undefined}
      extra="Surface data © OpenStreetMap contributors (ODbL)"
      choice={choice}
      onChoiceChange={setChoice}
    />
  );
}

const meta = {
  title: "Components/Map/Map Credits",
  component: MapCredits,
  tags: ["autodocs"],
  args: { styleUrl: undefined, choice: null, onChoiceChange: () => {} },
  decorators: [
    (Story) => (
      <StoryProviders>
        <div className="w-96 bg-[var(--base)] p-6">
          <Story />
        </div>
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof MapCredits>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Credits /> };
