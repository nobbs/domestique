/**
 * The field on real ground, which is the only place it can be judged: whether a
 * streak reads as air moving rather than as noise over the corridor, whether it
 * stays quiet enough for the route line to keep the eye, and whether the
 * direction can actually be taken off it without a legend.
 *
 * Nothing in a jsdom test can answer any of that. It draws through WebGL inside
 * MapLibre's own render pass, so this workshop and a screenshot of it are where
 * the drawing itself is looked at.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useEffect, useState } from "react";
import { weatherQuery } from "../../api/queries";
import type { BoundingBox, WeatherPoint } from "../../api/types";
import { CartographyProvider } from "../../components/map/CartographyContext";
import { MapViewport } from "../../components/map/MapViewport";
import { MapWidget } from "../../components/map/MapWidget";
import { coordinates, liveMap, weatherSamples } from "../../storybook/fixtures";
import { ConditionsWash } from "./ConditionsWash";
import { WindDriftField } from "./WindDriftField";
import { useWindRuns, WindRelationTint } from "./WindRelationTint";

const styles = {
  light: "https://tiles.openfreemap.org/styles/bright",
  dark: "https://tiles.openfreemap.org/styles/dark",
};

const bounds: BoundingBox = [7.995, 48.995, 8.045, 49.025];

/** A north-westerly that stiffens through the ride, so the field has a shape. */
const NORTH_WESTERLY: WeatherPoint[] = [315, 330, 340].map((windDirectionDegrees, index) => ({
  time: (weatherSamples[index]?.arrivalAt ?? new Date()).toISOString(),
  temperatureCelsius: 15,
  apparentTemperatureCelsius: 13,
  precipitationMillimetres: 0,
  precipitationProbabilityPercent: 10,
  windSpeedKmh: 14 + index * 10,
  windDirectionDegrees,
  weatherCode: 1,
  cloudCoverPercent: 25,
}));

function Forecast({ children }: { children: ReactNode }) {
  const [client] = useState(() => {
    const next = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
    });
    next.setQueryData(weatherQuery(weatherSamples).queryKey, { points: NORTH_WESTERLY });

    return next;
  });

  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

/**
 * The reduced-motion preference, for as long as the story below is mounted.
 *
 * Storybook has no way to ask the browser for it per story, and the whole point
 * of that branch is that it is chosen before anything is drawn — so the children
 * are held back until the preference reads the way this story needs it to, and
 * the real `matchMedia` is put back when the story goes.
 */
function StillAir({ children }: { children: ReactNode }) {
  const [ready, setReady] = useState(false);
  useEffect(() => {
    const real = window.matchMedia.bind(window);
    window.matchMedia = (query: string) =>
      query.includes("prefers-reduced-motion")
        ? ({
            matches: true,
            media: query,
            onchange: null,
            addEventListener: () => {},
            removeEventListener: () => {},
            addListener: () => {},
            removeListener: () => {},
            dispatchEvent: () => false,
          } as MediaQueryList)
        : real(query);
    setReady(true);

    return () => {
      window.matchMedia = real;
      setReady(false);
    };
  }, []);

  return ready ? <>{children}</> : null;
}

/** The three wind layers, exactly as `RouteOverlay` stacks them. */
function Wind({ washed = true, tinted = false }: { washed?: boolean; tinted?: boolean }) {
  const runs = useWindRuns(weatherSamples, coordinates, tinted);

  return (
    <>
      {washed ? (
        <ConditionsWash
          coordinates={coordinates}
          samples={weatherSamples}
          measure="wind"
          unitSystem="metric"
        />
      ) : null}
      <WindDriftField coordinates={coordinates} samples={weatherSamples} measure="wind" />
      {tinted ? (
        <WindRelationTint runs={runs} coordinates={coordinates} unitSystem="metric" />
      ) : null}
    </>
  );
}

function Blowing({
  dark = false,
  washed = true,
  tinted = false,
  still = false,
}: {
  dark?: boolean;
  washed?: boolean;
  tinted?: boolean;
  still?: boolean;
}) {
  const field = (
    <Forecast>
      <CartographyProvider dark={dark}>
        <MapWidget styleUrl={dark ? styles.dark : styles.light}>
          <MapViewport bounds={bounds} maxZoom={14} />
          <Wind washed={washed} tinted={tinted} />
        </MapWidget>
      </CartographyProvider>
    </Forecast>
  );

  return still ? <StillAir>{field}</StillAir> : field;
}

const meta = {
  title: "Components/Route/Wind Drift Field",
  parameters: liveMap,
  component: WindDriftField,
  tags: ["autodocs"],
  args: { coordinates, samples: weatherSamples, measure: "wind" as const },
  decorators: [
    (Story) => (
      <div className="h-[34rem] overflow-hidden rounded-xl">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof WindDriftField>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The field over the corridor it belongs to, which is how it is ever seen. */
export const Default: Story = { render: () => <Blowing /> };

/** On its own, with no wash under it: the streaks alone, at their true weight. */
export const WithoutTheCorridor: Story = { render: () => <Blowing washed={false} /> };

/** The ink follows the basemap, the same as every other mark on the map. */
export const DarkBasemap: Story = { render: () => <Blowing dark /> };

/**
 * All three of the wind's layers at once — speed in the corridor, direction in
 * the field, the rider's own share on the line. If the field competes with
 * either of the others, this is where it shows.
 */
export const WholeStack: Story = { render: () => <Blowing tinted /> };

/**
 * What a reader who asked for no movement gets instead: a dozen arrows standing
 * in the corridor, pointing where the air is going, and not one frame of
 * animation.
 */
export const ReducedMotion: Story = { render: () => <Blowing still /> };
