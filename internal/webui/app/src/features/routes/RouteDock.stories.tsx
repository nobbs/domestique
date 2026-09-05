/**
 * The dock, at the width the map's foot actually gives it.
 *
 * A rail of two stops on one distance axis, and the pill it leaves behind.
 * Everything between — switching stops, the departure moving — is reachable
 * from the first.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { userEvent } from "storybook/test";
import type { Highlight } from "../../lib/highlight";
import type { MeasureKey } from "../../lib/measures";
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
  const [measure, setMeasure] = useState<MeasureKey | null>(null);
  const [activeMetres, setActiveMetres] = useState<number | null>(null);
  const [zoomWindow, setZoomWindow] = useState<DistanceWindow | null>(null);
  // An hour before the first sample, and always inside the forecast window —
  // see `rideStart` for why a pinned date is not an option here.
  const [startAt, setStartAt] = useState<Date | null>(new Date(rideStart.getTime() - 60 * 60_000));

  return (
    <StoryProviders>
      <div className="flex justify-center bg-[var(--ground)] p-6">
        <RouteDock
          title={route.title}
          profile={profile}
          distanceMetres={route.distanceMetres}
          ascentMetres={route.ascentMetres}
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
          measure={measure}
          onMeasureChange={setMeasure}
          open={open}
          onOpenChange={setOpen}
        />
      </div>
    </StoryProviders>
  );
}

export const Open: Story = { render: () => <Docked startOpen /> };

/** Folded, which is most of the map back. */
export const Folded: Story = { render: () => <Docked startOpen={false} /> };

/** The forecast stop, reached by clicking its rail stop. */
export const Forecast: Story = {
  render: () => <Docked startOpen />,
  play: async ({ canvas }) => {
    await userEvent.click(canvas.getByRole("tab", { name: /Forecast/ }));
  },
};
