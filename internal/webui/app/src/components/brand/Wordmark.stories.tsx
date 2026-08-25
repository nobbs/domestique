import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../storybook/fixtures";
import { Wordmark } from "./Wordmark";

const meta = {
  title: "Components/Brand/Wordmark",
  component: Wordmark,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <StoryProviders>
        <div className="bg-[var(--base)] p-6">
          <Story />
        </div>
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof Wordmark>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Ready: Story = {};
