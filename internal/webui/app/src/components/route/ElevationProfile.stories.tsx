import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { Highlight } from "../../lib/highlight";
import { profile, surface } from "../../storybook/fixtures";
import { ElevationProfile } from "./ElevationProfile";

function Profile() {
  const [activeMetres, setActiveMetres] = useState<number | null>(null);
  const highlight: Highlight = { type: "band", band: 3 };

  return (
    <ElevationProfile
      profile={profile}
      title="Alpine loop"
      surface={surface}
      activeMetres={activeMetres}
      onActiveChange={setActiveMetres}
      highlight={highlight}
      onZoomChange={() => {}}
    />
  );
}

const meta = {
  title: "Components/Route/Elevation Profile",
  component: ElevationProfile,
  tags: ["autodocs"],
  args: { profile: null, title: "", activeMetres: null, onActiveChange: () => {} },
  decorators: [
    (Story) => (
      <div className="max-w-3xl bg-[var(--base)] p-6">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof ElevationProfile>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Profile /> };
