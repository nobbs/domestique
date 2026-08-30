import type { Meta, StoryObj } from "@storybook/react-vite";
import { useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { Marker } from "react-map-gl/maplibre";
import { weatherQuery } from "../../api/queries";
import type { BoundingBox } from "../../api/types";
import { CartographyProvider } from "../../components/map/CartographyContext";
import { MapViewport } from "../../components/map/MapViewport";
import { MapWidget } from "../../components/map/MapWidget";
import type { ProfileSample } from "../../lib/profile";
import { sampleAt } from "../../lib/profile";
import {
  coordinates,
  liveMap,
  profile as maybeProfile,
  StoryProviders,
  surface,
  weatherSamples,
} from "../../storybook/fixtures";
import { PositionTooltip } from "./PositionTooltip";

const styleUrl = "https://tiles.openfreemap.org/styles/bright";
const darkStyleUrl = "https://tiles.openfreemap.org/styles/dark";
const bounds: BoundingBox = [7.995, 48.995, 8.045, 49.025];

// The fixture coordinates always build a profile; narrowed once here so the
// stories below can read `endMetres` and pick samples without threading null.
if (!maybeProfile) {
  throw new Error("the storybook fixture coordinates should build a profile");
}
const profile = maybeProfile;

function sample(metres: number): ProfileSample {
  const found = sampleAt(profile, metres);
  if (!found) {
    throw new Error(`no sample at ${metres}m`);
  }

  return found;
}

const midway = sample(profile.endMetres / 2);

/**
 * A forecast for the fixture's own samples, seeded rather than fetched.
 *
 * Where the wind blows from is what decides the relation, so a story asks for a
 * headwind or a tailwind by choosing that rather than by faking the reading —
 * the classification stays the shipping code path.
 */
function SeedForecast({ fromDegrees, children }: { fromDegrees: number; children: ReactNode }) {
  const client = useQueryClient();
  // Seeded once on mount rather than on every render, the way `StoryProviders`
  // seeds its own: writing to the cache while rendering is a side effect, and
  // under StrictMode it is one that repeats.
  useState(() => {
    client.setQueryData(weatherQuery(weatherSamples).queryKey, {
      points: weatherSamples.map((entry) => ({
        time: entry.arrivalAt.toISOString(),
        temperatureCelsius: 18,
        apparentTemperatureCelsius: 17,
        precipitationMillimetres: 0,
        precipitationProbabilityPercent: 5,
        windSpeedKmh: 18,
        windDirectionDegrees: fromDegrees,
        weatherCode: 1,
      })),
    });
  });

  return children;
}

/**
 * Stands in for `RouteOverlay`'s `route-position-dot` layer, which is drawn
 * into the canvas and so is not here. Without it there is nothing for the arrow
 * to be seen pointing at.
 */
function Dot({ dark }: { dark: boolean }) {
  return (
    <Marker
      longitude={midway.longitude}
      latitude={midway.latitude}
      className="route-position-tooltip-marker"
    >
      <span
        className="block size-2.5 rounded-full"
        style={{ background: dark ? "#70adfb" : "#236fc7" }}
      />
    </Marker>
  );
}

function OnMap({
  children,
  style = styleUrl,
  dark = false,
}: {
  children: ReactNode;
  style?: string;
  dark?: boolean;
}) {
  return (
    <CartographyProvider dark={dark}>
      <MapWidget styleUrl={style}>
        <MapViewport bounds={bounds} maxZoom={14} />
        <Dot dark={dark} />
        {children}
      </MapWidget>
    </CartographyProvider>
  );
}

const meta = {
  title: "Components/Route/Position Tooltip",
  parameters: liveMap,
  component: PositionTooltip,
  tags: ["autodocs"],
  args: {
    position: midway,
    content: midway,
    endMetres: profile.endMetres,
    surfaceSummary: surface,
    coordinates,
    samples: [],
    announce: false,
    unitSystem: "metric",
  },
  decorators: [
    (Story) => (
      <StoryProviders>
        <div className="h-[34rem] overflow-hidden rounded-xl">
          <Story />
        </div>
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof PositionTooltip>;

export default meta;
type Story = StoryObj<typeof meta>;

/** No start time picked, so no forecast and no second row: one line and the bar. */
export const Default: Story = {
  render: (args) => (
    <OnMap>
      <PositionTooltip {...args} />
    </OnMap>
  ),
};

/** A wind behind the rider here, which the arrow runs away from the reader to say. */
export const Tailwind: Story = {
  render: (args) => (
    <SeedForecast fromDegrees={250}>
      <OnMap>
        <PositionTooltip {...args} samples={weatherSamples} />
      </OnMap>
    </SeedForecast>
  ),
};

/** The same wind from the opposite quarter: the arrow turns back, and reddens. */
export const Headwind: Story = {
  render: (args) => (
    <SeedForecast fromDegrees={70}>
      <OnMap>
        <PositionTooltip {...args} samples={weatherSamples} />
      </OnMap>
    </SeedForecast>
  ),
};

/**
 * A wind square across the road, which pushes the rider neither way — so the
 * figure is the wind's own speed rather than a component of nought.
 */
export const Crosswind: Story = {
  render: (args) => (
    <SeedForecast fromDegrees={160}>
      <OnMap>
        <PositionTooltip {...args} samples={weatherSamples} />
      </OnMap>
    </SeedForecast>
  ),
};

/** On dark cartography every colour swaps, the box included. */
export const OnDarkCartography: Story = {
  render: (args) => (
    <SeedForecast fromDegrees={250}>
      <OnMap style={darkStyleUrl} dark={true}>
        <PositionTooltip {...args} samples={weatherSamples} />
      </OnMap>
    </SeedForecast>
  ),
};

/** Carrying the announcement itself, for a reader who has folded the profile away. */
export const Announcing: Story = {
  render: (args) => (
    <OnMap>
      <PositionTooltip {...args} announce={true} />
    </OnMap>
  ),
};
