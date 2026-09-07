import type { Meta, StoryObj } from "@storybook/react-vite";
import { useMemo, useState } from "react";
import type { Highlight } from "../../lib/highlight";
import type { AlignedSeries } from "../../lib/rideSeries";
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

/**
 * The same chart as a ride page draws it: sensor series over the terrain, each
 * on its own hidden axis. The readings here are made up from the ground the
 * fixture covers, since a fixture holding a real ride's sensors would be a
 * rider's own data.
 */
function WithSeries() {
  const [activeMetres, setActiveMetres] = useState<number | null>(null);
  const series = useMemo((): AlignedSeries[] => {
    const samples = profile?.samples ?? [];

    return [
      {
        key: "heartRate",
        label: "Heart rate",
        unit: "bpm",
        colour: "var(--series-heart-rate)",
        values: samples.map(
          (sample, index) => 132 + sample.gradientPercent * 3 + Math.sin(index / 34) * 9,
        ),
      },
      {
        key: "cadence",
        label: "Cadence",
        unit: "rpm",
        colour: "var(--series-cadence)",
        values: samples.map((sample, index) =>
          index % 17 === 0 ? null : 86 - sample.gradientPercent * 2 + Math.cos(index / 21) * 7,
        ),
      },
      {
        key: "temperature",
        label: "Temperature",
        unit: "°C",
        colour: "var(--series-temperature)",
        values: samples.map(
          (sample, index) => 24 - sample.elevationMetres / 160 + Math.sin(index / 40),
        ),
      },
    ];
  }, []);

  return (
    <ElevationProfile
      profile={profile}
      title="Alpine loop"
      series={series}
      activeMetres={activeMetres}
      onActiveChange={setActiveMetres}
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

export const WithSensorSeries: Story = { render: () => <WithSeries /> };
