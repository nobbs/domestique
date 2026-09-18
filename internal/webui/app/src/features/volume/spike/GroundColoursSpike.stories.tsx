/**
 * Colour pairs for outdoor and indoor on the real volume page. Each story only
 * overrides the two ground tokens; switch the toolbar theme to see the dark pair.
 *
 * Storybook only, over two synthetic years of mixed riding.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { activitiesQuery } from "../../../api/queries";
import type { Activity } from "../../../api/types";
import { StoryProviders } from "../../../storybook/fixtures";
import { ActivitiesPage } from "../../activity/ActivitiesPage";

const DAY = 86_400_000;
const NOW = Date.now();

const RIDES: Activity[] = Array.from({ length: 720 }, (_, daysAgo) => daysAgo).flatMap(
  (daysAgo) => {
    const date = new Date(NOW - daysAgo * DAY);
    const weekday = (date.getUTCDay() + 6) % 7;
    const summer = Math.sin(((date.getUTCMonth() - 2) / 12) * 2 * Math.PI);
    // Winter moves the weekday rides indoors; weekends stay out when they can.
    const indoor = summer < -0.2 ? weekday !== 6 : summer < 0.3 && weekday < 5;
    if (![1, 3, 5, 6].includes(weekday) || daysAgo % 13 === 0) {
      return [];
    }
    const distanceMetres = Math.round(
      (indoor ? 32_000 : 55_000) * (weekday === 6 ? 1.9 : 1) * (1 + 0.2 * Math.sin(daysAgo)),
    );
    return [
      {
        id: `ride-${daysAgo}`,
        startedAt: new Date(date.setUTCHours(7, 0, 0, 0)).toISOString(),
        distanceMetres,
        movingSeconds: Math.round(distanceMetres / (indoor ? 8.6 : 7.6)),
        elapsedSeconds: Math.round(distanceMetres / 7),
        ascentMetres: Math.round((distanceMetres / 1000) * (indoor ? 6 : 11)),
        typeId: indoor ? 61 : 0,
        locationId: 0,
        indoor,
        provider: "wahoo",
      },
    ];
  },
);

function Seeded({ children }: { children: ReactNode }) {
  const client = useQueryClient();
  useState(() => client.setQueryData(activitiesQuery().queryKey, RIDES));
  return children;
}

interface Pair {
  outdoor: [light: string, dark: string];
  indoor: [light: string, dark: string];
}

function palette({ outdoor, indoor }: Pair): Story {
  return {
    render: (_, { globals }) => {
      const dark = globals.theme === "dark" ? 1 : 0;
      return (
        <StoryProviders>
          <Seeded>
            <div
              style={
                {
                  "--ground-outdoor": outdoor[dark],
                  "--ground-indoor": indoor[dark],
                } as React.CSSProperties
              }
            >
              <ActivitiesPage />
            </div>
          </Seeded>
        </StoryProviders>
      );
    },
  };
}

const meta = {
  title: "Spikes/Volume Ground Colours",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A · Moss and violet: the first pair the page carried. */
export const MossViolet = palette({
  outdoor: ["#3f7d4c", "#86c792"],
  indoor: ["#6f5cc2", "#aa9cf2"],
});

/** B · Terracotta and slate: earth outside, a cool room inside; sits on the warm greige ground. */
export const TerracottaSlate = palette({
  outdoor: ["#b5643c", "#e39a73"],
  indoor: ["#4f6d8a", "#93afcb"],
});

/** C · Olive and warm grey: outdoor is the colour, indoor recedes as the lesser riding. */
export const OliveGrey = palette({
  outdoor: ["#5f7f2e", "#a9c96f"],
  indoor: ["#a39a8c", "#7d766c"],
});

/** D · Blue and amber: the strongest contrast, and safe for red-green colour blindness. */
export const BlueAmber = palette({
  outdoor: ["#2f6fb3", "#7fb2eb"],
  indoor: ["#d08a1e", "#f0b862"],
});

/** E · Teal and plum: two quiet mid-tones of equal weight. */
export const TealPlum = palette({
  outdoor: ["#1f7f72", "#6fcfbf"],
  indoor: ["#8e4a7a", "#d59ac4"],
});

/** F · Ink and sand: near-black outdoor like the page's primary, a pale sand for indoor. */
export const InkSand = palette({
  outdoor: ["#2d2b28", "#e8e4dc"],
  indoor: ["#cdb58c", "#8f7a55"],
});

/** G · Olive and slate: C's outdoor beside B's indoor; what the page carries now. */
export const OliveSlate = palette({
  outdoor: ["#5f7f2e", "#a9c96f"],
  indoor: ["#4f6d8a", "#93afcb"],
});
