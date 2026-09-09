/**
 * What should stand where "By the kilometre" stands on the ride page (#646).
 *
 * Storybook only: nothing here is imported by the application. Each variant
 * runs against three ride shapes — flat, hilly and matched, and unmatched —
 * so an empty or thin case is seen beside the ordinary one, not asserted in
 * prose. See `splitsVariants.tsx` for what each variant draws and the bet it
 * makes.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { StoryProviders } from "../../../storybook/fixtures";
import { RIDES, type SplitsRideFixture } from "./splitsData";
import { Baseline, ByClimb, ByTerrain, DropRegion, Pacing } from "./splitsVariants";

const meta = {
  title: "Spikes/Ride Splits",
  parameters: { chromatic: { disableSnapshot: true } },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

function RideCard({ ride, children }: { ride: SplitsRideFixture; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-2">
      <h3 className="font-medium text-[var(--ink-2)] text-xs uppercase tracking-wide">
        {ride.label}
      </h3>
      {children}
    </div>
  );
}

interface VariantProps {
  ride: SplitsRideFixture;
  activeMetres?: number | null;
  onActiveChange?: (metres: number | null) => void;
}

function page(bet: string, Variant: (props: VariantProps) => React.JSX.Element | null): Story {
  return {
    render: () => (
      <StoryProviders>
        <div className="flex min-h-dvh flex-col gap-6 bg-[var(--base)] p-6">
          <p className="max-w-2xl text-[var(--ink-2)] text-sm">{bet}</p>
          {RIDES.map((ride) => (
            <RideCard key={ride.key} ride={ride}>
              <Variant ride={ride} />
            </RideCard>
          ))}
        </div>
      </StoryProviders>
    ),
  };
}

/** Today's panel, for comparison against every variant below. */
export const BaselineToday = page(
  "Today: a bar per fixed kilometre, a table behind a toggle.",
  Baseline,
);

/**
 * A · The test is whether anything the splits say is not already readable
 * from the series chart, the effort panel and the profile. If nothing is,
 * deletion wins.
 */
export const Drop = page(
  "Bet: nothing here says anything the series chart, the effort panel and the profile don't already say. If that holds, delete the panel.",
  DropRegion,
);

/**
 * B · The ride's attempts on its route's sustained climbs, each beside the
 * rider's other attempts at it. Needs a read scoped to the activity, or a
 * client-side filter over the route's own climbs-with-attempts read.
 */
export const ByTheClimb = page(
  "Bet: a rider wants to know how the climb went, not how the third kilometre went. Needs the route's climb attempts, filtered to this ride.",
  ByClimb,
);

/**
 * C · Stretches cut where the profile's gradient band changes, not every
 * kilometre — comparable across two rides of the same route because the
 * cuts follow the ground.
 */
export const ByTheTerrain = page(
  "Bet: the ground, not the odometer, should decide where one stretch ends and the next begins.",
  ByTerrain,
);

/**
 * D · First half against second, or thirds — three rows, and the decoupling
 * between them.
 */
export const PacingStory = page(
  "Bet: three parts and a decoupling figure say more about the ride than fifty kilometre rows do.",
  Pacing,
);
PacingStory.storyName = "Pacing";

/**
 * Hover sync check: `activeMetres` plumbed into `ByTerrain` as it would be
 * from a map and a profile sharing one cursor. Not a fifth position — this
 * exists to show the wiring works, so it is one ride rather than three.
 */
export const ByTheTerrainHoverSync: Story = {
  render: () => {
    function Demo() {
      const [activeMetres, setActiveMetres] = useState<number | null>(null);
      const ride = RIDES[1] as SplitsRideFixture;

      return (
        <div className="flex min-h-dvh flex-col gap-6 bg-[var(--base)] p-6">
          <p className="max-w-2xl text-[var(--ink-2)] text-sm">
            Hovering the bars moves `activeMetres` — the position a map and a profile would share
            with this panel on the real page.
          </p>
          <RideCard ride={ride}>
            <ByTerrain ride={ride} activeMetres={activeMetres} onActiveChange={setActiveMetres} />
          </RideCard>
        </div>
      );
    }

    return (
      <StoryProviders>
        <Demo />
      </StoryProviders>
    );
  },
};
