import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { weatherQuery } from "../../api/queries";
import type { BoundingBox, WeatherPoint } from "../../api/types";
import { CartographyProvider } from "../../components/map/CartographyContext";
import { MapViewport } from "../../components/map/MapViewport";
import { MapWidget } from "../../components/map/MapWidget";
import { buildProfile } from "../../lib/profile";
import { coordinates, liveMap, weatherSamples } from "../../storybook/fixtures";
import { RouteOverlay } from "./RouteOverlay";

const styleUrl = "https://tiles.openfreemap.org/styles/bright";
const bounds: BoundingBox = [7.995, 48.995, 8.045, 49.025];
const profile = buildProfile(coordinates);

const meta = {
  title: "Components/Route/Route Overlay",
  parameters: liveMap,
  component: RouteOverlay,
  tags: ["autodocs"],
  args: { coordinates },
  decorators: [
    (Story) => (
      <div className="h-[34rem] overflow-hidden rounded-xl">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof RouteOverlay>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => (
    <CartographyProvider dark={false}>
      <MapWidget styleUrl={styleUrl}>
        <MapViewport bounds={bounds} maxZoom={14} />
        <RouteOverlay coordinates={coordinates} profile={profile} activeProfile={profile} />
      </MapWidget>
    </CartographyProvider>
  ),
};

/** A wind that swings round through the ride, so the tint has both ends to draw. */
const SWINGING_WIND: WeatherPoint[] = [250, 300, 20].map((windDirectionDegrees, index) => ({
  time: (weatherSamples[index]?.arrivalAt ?? new Date()).toISOString(),
  temperatureCelsius: 15,
  apparentTemperatureCelsius: 13,
  precipitationMillimetres: 0,
  precipitationProbabilityPercent: 10,
  windSpeedKmh: 12 + index * 12,
  windDirectionDegrees,
  weatherCode: 1,
  cloudCoverPercent: 25,
}));

/**
 * The wind measure, where the route carries what the wind is doing to the rider
 * and the steepness edging stands down for it — the one thing to look for here
 * is that no stretch is wearing two ramps at once.
 */
export const WindTinted: Story = {
  render: function WindTintedStory() {
    const [client] = useState(() => {
      const next = new QueryClient({
        defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
      });
      next.setQueryData(weatherQuery(weatherSamples).queryKey, { points: SWINGING_WIND });

      return next;
    });

    return (
      <QueryClientProvider client={client}>
        <CartographyProvider dark={false}>
          <MapWidget styleUrl={styleUrl}>
            <MapViewport bounds={bounds} maxZoom={14} />
            <RouteOverlay
              coordinates={coordinates}
              profile={profile}
              activeProfile={profile}
              samples={weatherSamples}
              measure="wind"
            />
          </MapWidget>
        </CartographyProvider>
      </QueryClientProvider>
    );
  },
};
