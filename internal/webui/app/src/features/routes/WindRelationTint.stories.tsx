/**
 * The tint on real ground, which is the only place it can be judged: whether a
 * headwind and a tailwind are told apart at a glance, whether the neutral for a
 * shifting stretch reads as no answer rather than as a middling one, and whether
 * the ramp stays clear of the corridor washed around it.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useState } from "react";
import { weatherQuery } from "../../api/queries";
import type { BoundingBox, WeatherPoint } from "../../api/types";
import { CartographyProvider } from "../../components/map/CartographyContext";
import { MapViewport } from "../../components/map/MapViewport";
import { MapWidget } from "../../components/map/MapWidget";
import { coordinates, liveMap, weatherSamples } from "../../storybook/fixtures";
import { ConditionsWash } from "./ConditionsWash";
import { useWindRuns, WindRelationTint } from "./WindRelationTint";

const styles = {
  light: "https://tiles.openfreemap.org/styles/bright",
  dark: "https://tiles.openfreemap.org/styles/dark",
};

const bounds: BoundingBox = [7.995, 48.995, 8.045, 49.025];

/** A wind that swings round through the ride, so the ramp has both ends to draw. */
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

function Forecast({ children }: { children: ReactNode }) {
  const [client] = useState(() => {
    const next = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
    });
    next.setQueryData(weatherQuery(weatherSamples).queryKey, { points: SWINGING_WIND });

    return next;
  });

  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

/** The tint reading its own stretches, exactly as the overlay hands them to it. */
function Tint({ withWash = false }: { withWash?: boolean }) {
  const runs = useWindRuns(weatherSamples, coordinates, true);

  return (
    <>
      {withWash ? (
        <ConditionsWash coordinates={coordinates} samples={weatherSamples} measure="wind" />
      ) : null}
      <WindRelationTint runs={runs} coordinates={coordinates} />
    </>
  );
}

function Tinted({ dark = false, withWash = false }: { dark?: boolean; withWash?: boolean }) {
  return (
    <Forecast>
      <CartographyProvider dark={dark}>
        <MapWidget styleUrl={dark ? styles.dark : styles.light}>
          <MapViewport bounds={bounds} maxZoom={14} />
          <Tint withWash={withWash} />
        </MapWidget>
      </CartographyProvider>
    </Forecast>
  );
}

const meta = {
  title: "Components/Route/Wind Relation Tint",
  parameters: liveMap,
  component: WindRelationTint,
  tags: ["autodocs"],
  args: { runs: [], coordinates },
  decorators: [
    (Story) => (
      <div className="h-[34rem] overflow-hidden rounded-xl">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof WindRelationTint>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The ramp on its own, head to tail along a ride the wind swings round on. */
export const Default: Story = { render: () => <Tinted /> };

/** The ramp follows the basemap, not the page, the same as every mark on the map. */
export const DarkBasemap: Story = { render: () => <Tinted dark /> };

/**
 * Both halves of the wind at once: the corridor for how hard it blows, the route
 * for what that does to the rider. The two ramps have to stay tellable apart.
 */
export const OverTheCorridor: Story = { render: () => <Tinted withWash /> };
