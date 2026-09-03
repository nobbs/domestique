/**
 * The wash on real ground, which is the only place it can be judged: whether
 * the corridor hugs the route, whether its edge really has no edge, and whether
 * a band change reads as a change in the weather rather than as a seam.
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
import type { MeasureKey } from "../../lib/measures";
import { coordinates, liveMap, weatherSamples } from "../../storybook/fixtures";
import { ConditionsWash } from "./ConditionsWash";

const styles = {
  light: "https://tiles.openfreemap.org/styles/bright",
  dark: "https://tiles.openfreemap.org/styles/dark",
};

/** Pulled back far enough that the corridor's own width fits on screen. */
const bounds: BoundingBox = [7.95, 48.97, 8.09, 49.05];

/**
 * A front arriving over the ride: dry at the start, soaked by the finish, so
 * the wash has every band to draw rather than one flat colour.
 */
const ARRIVING_FRONT: WeatherPoint[] = [0, 1.2, 7].map((millimetres, index) => ({
  time: (weatherSamples[index]?.arrivalAt ?? new Date()).toISOString(),
  temperatureCelsius: 16 - index * 4,
  apparentTemperatureCelsius: 15 - index * 5,
  precipitationMillimetres: millimetres,
  precipitationProbabilityPercent: index * 40,
  windSpeedKmh: 8 + index * 18,
  windDirectionDegrees: 250,
  weatherCode: index === 0 ? 1 : 61,
  cloudCoverPercent: 10 + index * 40,
}));

function Forecast({ children }: { children: ReactNode }) {
  const [client] = useState(() => {
    const next = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
    });
    next.setQueryData(weatherQuery(weatherSamples).queryKey, { points: ARRIVING_FRONT });

    return next;
  });

  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

function Washed({ measure, dark = false }: { measure: MeasureKey; dark?: boolean }) {
  return (
    <Forecast>
      <CartographyProvider dark={dark}>
        <MapWidget styleUrl={dark ? styles.dark : styles.light}>
          <MapViewport bounds={bounds} maxZoom={12} />
          <ConditionsWash
            coordinates={coordinates}
            samples={weatherSamples}
            measure={measure}
            unitSystem="metric"
          />
        </MapWidget>
      </CartographyProvider>
    </Forecast>
  );
}

const meta = {
  title: "Components/Route/Conditions Wash",
  parameters: liveMap,
  component: ConditionsWash,
  tags: ["autodocs"],
  args: { coordinates, samples: weatherSamples, measure: "rain" },
  decorators: [
    (Story) => (
      <div className="h-[34rem] overflow-hidden rounded-xl">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof ConditionsWash>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Rain, whose dry band paints nothing at all — the wash starts where the rain does. */
export const Rain: Story = { render: () => <Washed measure="rain" /> };

/** Temperature, which has something to say everywhere and so washes end to end. */
export const Temperature: Story = { render: () => <Washed measure="temperature" /> };

/** The ramp follows the basemap, not the page, the same as every other mark on the map. */
export const DarkBasemap: Story = { render: () => <Washed measure="temperature" dark /> };
