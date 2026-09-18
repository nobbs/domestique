/**
 * Three wide layouts for the ride page after the fitness page's strip and rail.
 *
 * Storybook only. The cards are the real ones over the spike's synthetic ride;
 * the map and the profile are stand-ins sized as the page draws them.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { IconMountain } from "@tabler/icons-react";
import type { ReactNode } from "react";
import type { Activity, RouteClimb } from "../../../api/types";
import { PanelHeading } from "../../../components/PanelHeading";
import { StoryProviders } from "../../../storybook/fixtures";
import { HeartRateZones } from "../HeartRateZones";
import { RideAnalysis } from "../RideAnalysis";
import { RideClimbs } from "../RideClimbs";
import { HillyRide } from "../RideClimbs.stories";
import { RideFigures } from "../RideFigures";
import { TrainingLoad } from "../TrainingLoad";
import { ASCENT_METRES, ELAPSED_SECONDS, METRICS, MOVING_SECONDS, TOTAL_METRES } from "./data";

const RIDE: Activity = {
  id: "42",
  startedAt: "2026-09-06T08:00:00Z",
  distanceMetres: TOTAL_METRES,
  movingSeconds: MOVING_SECONDS,
  elapsedSeconds: ELAPSED_SECONDS,
  ascentMetres: ASCENT_METRES,
  typeId: 0,
  locationId: 0,
  indoor: false,
  provider: "wahoo",
  metrics: METRICS,
  analysis: {
    text: "A steady endurance ride with the effort held back on the first climb and spent on the last. Heart rate drifted little for the power held, which says the aerobic base is carrying the load.\n\nThe second half averaged 6 W more than the first at the same heart rate.",
    model: "claude-sonnet-5",
    promptRevision: 1,
    analysedAt: "2026-09-06T14:00:00Z",
  },
};

// Without heart-rate zones the effort card draws only its sensors.
const SENSORS_ONLY: Activity = {
  ...RIDE,
  metrics: { ...METRICS, zoneSeconds: [0, 0, 0, 0, 0] },
};

const CLIMBS = (HillyRide.args?.climbs ?? []) as RouteClimb[];
const BOX = "flex min-w-0 flex-col gap-4 rounded-2xl bg-[var(--panel)] p-5 shadow-[var(--shadow)]";

const Hero = () => (
  <div className="grid gap-4 rounded-2xl bg-[var(--panel)] p-5 shadow-[var(--shadow)]">
    <div>
      <h1 className="font-semibold text-lg leading-tight">Kaiserstuhl Loop</h1>
      <p className="text-[var(--ink-2)] text-sm">6 Sept 2026, 10:00 · 11–22°, wind 17 km/h</p>
    </div>
    <RideFigures ride={RIDE} />
  </div>
);

function Stand({ label, className }: { label: string; className: string }) {
  return (
    <div
      className={`grid place-items-center rounded-2xl border border-[var(--rule)] bg-[repeating-linear-gradient(45deg,var(--panel-2,#ebe8e2)_0_12px,transparent_12px_24px)] text-[var(--ink-2)] text-sm shadow-[var(--shadow)] ${className}`}
    >
      {label}
    </div>
  );
}

const MapStand = ({ className = "h-80" }: { className?: string }) => (
  <Stand label="Map" className={className} />
);

const Profile = () => (
  <section className={BOX}>
    <PanelHeading icon={<IconMountain size={18} stroke={1.8} />} title="Profile" />
    <Stand label="Elevation, series and conditions" className="h-72 shadow-none" />
  </section>
);

const Heart = () => (
  <section className={BOX} aria-label="Heart rate">
    <HeartRateZones
      rideId={RIDE.id}
      zoneSeconds={METRICS.zoneSeconds ?? []}
      zoneBounds={METRICS.zoneBoundsBpm}
      deviceZoneSeconds={undefined}
      coverage={undefined}
    />
  </section>
);

const Sensors = () => <TrainingLoad ride={SENSORS_ONLY} />;
const Climbs = () => <RideClimbs climbs={CLIMBS} activityId={RIDE.id} />;
const Analysis = () => <RideAnalysis ride={RIDE} />;

function Page({ children }: { children: ReactNode }) {
  return (
    <StoryProviders>
      <div className="min-h-dvh bg-[var(--base)] px-6 py-8 text-[var(--ink)]">
        <div className="mx-auto flex w-full max-w-[1400px] flex-col gap-5">{children}</div>
      </div>
    </StoryProviders>
  );
}

const Rail = ({ children, width = "22rem" }: { children: ReactNode; width?: string }) => (
  <div
    className="grid items-start gap-5 lg:grid-cols-[minmax(0,1fr)_var(--rail)]"
    style={{ "--rail": width } as React.CSSProperties}
  >
    {children}
  </div>
);

const Column = ({ children, className = "" }: { children: ReactNode; className?: string }) => (
  <div className={`flex min-w-0 flex-col gap-5 ${className}`}>{children}</div>
);

const meta = {
  title: "Spikes/Activity Layout",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A · Fitness's pattern: hero across; map, profile, climbs and analysis in the main column; heart rate and sensors in the rail. */
export const StripAndRail: Story = {
  render: () => (
    <Page>
      <Hero />
      <Rail>
        <Column>
          <MapStand />
          <Profile />
          <Climbs />
          <Analysis />
        </Column>
        <Column>
          <Heart />
          <Sensors />
        </Column>
      </Rail>
    </Page>
  ),
};

/** B · The body in the rail: heart rate, sensors and climbs beside map and profile; analysis reads across the foot. */
export const BodyInRail: Story = {
  render: () => (
    <Page>
      <Hero />
      <Rail width="26rem">
        <Column>
          <MapStand className="h-[28rem]" />
          <Profile />
        </Column>
        <Column>
          <Heart />
          <Sensors />
          <Climbs />
        </Column>
      </Rail>
      <Analysis />
    </Page>
  ),
};

/** C · Map in the rail: a tall map held beside the hero and the numbers, as a ride log reads. */
export const MapInRail: Story = {
  render: () => (
    <Page>
      <Rail width="30rem">
        <Column>
          <Hero />
          <Profile />
          <div className="grid items-start gap-5 xl:grid-cols-2">
            <Heart />
            <Sensors />
          </div>
          <Climbs />
          <Analysis />
        </Column>
        <Column className="lg:sticky lg:top-6">
          <MapStand className="h-[40rem]" />
        </Column>
      </Rail>
    </Page>
  ),
};
