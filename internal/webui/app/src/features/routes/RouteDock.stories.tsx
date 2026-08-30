/**
 * The dock, at the width the map's foot actually gives it.
 *
 * Two states, because they are the two things it is: the three lanes on one
 * distance axis, and the pill it leaves behind. Everything between — the ground
 * key folding, the forecast folding, the departure moving — is reachable from
 * the first.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { Highlight } from "../../lib/highlight";
import type { DistanceWindow } from "../../lib/profile";
import {
  climbs,
  coordinates,
  profile,
  rideStart,
  route,
  StoryProviders,
  surface,
  weatherSamples,
} from "../../storybook/fixtures";
import { RouteDock } from "./RouteDock";

const meta = {
  title: "Features/Atlas/Route Dock",
  // No snapshot: the picker prints the ride's day, and the day is relative to
  // the real clock — a pinned one drifts out of the forecast window, and a
  // relative one re-diffs every build. Interactions are still tested.
  parameters: { layout: "fullscreen", chromatic: { disableSnapshot: true } },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

function Docked({ startOpen }: { startOpen: boolean }) {
  const [open, setOpen] = useState(startOpen);
  const [highlight, setHighlight] = useState<Highlight | null>(null);
  const [activeMetres, setActiveMetres] = useState<number | null>(null);
  const [zoomWindow, setZoomWindow] = useState<DistanceWindow | null>(null);
  // An hour before the first sample, and always inside the forecast window —
  // see `rideStart` for why a pinned date is not an option here.
  const [startAt, setStartAt] = useState<Date | null>(new Date(rideStart.getTime() - 60 * 60_000));
  const [groundLabelled, setGroundLabelled] = useState(true);
  const [forecastOpen, setForecastOpen] = useState(true);

  return (
    <StoryProviders>
      <div className="flex justify-center bg-[var(--ground)] p-6">
        <RouteDock
          title={route.title}
          profile={profile}
          distanceMetres={route.distanceMetres}
          surface={surface}
          climbs={climbs}
          onSelectClimb={() => {}}
          coordinates={coordinates}
          samples={weatherSamples}
          startAt={startAt}
          onStartAtChange={setStartAt}
          movingSeconds={route.movingSeconds}
          activeMetres={activeMetres}
          onActiveChange={setActiveMetres}
          zoomWindow={zoomWindow}
          onZoomChange={setZoomWindow}
          highlight={highlight}
          onHighlightChange={setHighlight}
          unitSystem="metric"
          open={open}
          onOpenChange={setOpen}
          groundLabelled={groundLabelled}
          onGroundLabelledChange={setGroundLabelled}
          forecastOpen={forecastOpen}
          onForecastOpenChange={setForecastOpen}
        />
      </div>
    </StoryProviders>
  );
}

export const Open: Story = { render: () => <Docked startOpen /> };

/** Folded, which is most of the map back. */
export const Folded: Story = { render: () => <Docked startOpen={false} /> };
