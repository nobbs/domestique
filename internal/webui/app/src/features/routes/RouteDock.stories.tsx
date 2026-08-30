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
  route,
  StoryProviders,
  surface,
  weatherSamples,
} from "../../storybook/fixtures";
import { RouteDock } from "./RouteDock";

const meta = {
  title: "Features/Atlas/Route Dock",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

function Docked({ startOpen }: { startOpen: boolean }) {
  const [open, setOpen] = useState(startOpen);
  const [highlight, setHighlight] = useState<Highlight | null>(null);
  const [activeMetres, setActiveMetres] = useState<number | null>(null);
  const [zoomWindow, setZoomWindow] = useState<DistanceWindow | null>(null);
  const [startAt, setStartAt] = useState<Date | null>(new Date("2026-08-18T06:00:00Z"));
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
