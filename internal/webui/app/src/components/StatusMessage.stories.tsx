import type { Meta, StoryObj } from "@storybook/react-vite";
import { ErrorMessage, LoadingMessage, StatusMessage } from "./StatusMessage";

const meta = {
  title: "Components/Status Message",
  component: StatusMessage,
  tags: ["autodocs"],
  args: { title: "Status" },
  decorators: [
    (Story) => (
      <div className="max-w-md bg-[var(--base)] p-6">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof StatusMessage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Neutral: Story = {
  args: { title: "No routes match", detail: "Try clearing a filter." },
};

export const Loading: Story = { render: () => <LoadingMessage what="the route library" /> };

export const Failure: Story = {
  render: () => <ErrorMessage what="the route library" error={new Error("Service unavailable")} />,
};
