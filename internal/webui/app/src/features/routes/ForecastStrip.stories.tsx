import type { Meta, StoryObj } from "@storybook/react-vite";
import { coordinates, StoryProviders, weatherSamples } from "../../storybook/fixtures";
import { ForecastStrip } from "./ForecastStrip";

const meta = {
  title: "Components/Route/Forecast Strip",
  component: ForecastStrip,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <StoryProviders>
        <div className="max-w-3xl bg-[var(--base)] p-6">
          <Story />
        </div>
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof ForecastStrip>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    samples: weatherSamples,
    coordinates,
    startMetres: 0,
    endMetres: 3_000,
  },
};
