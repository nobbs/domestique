/**
 * The credits card, with the tile attribution seeded rather than fetched.
 *
 * A story must not reach a tile provider, so the query the card reads is filled
 * in here. What the story shows is the arrangement — one row per source, the
 * configured basemaps first — rather than any real provider's wording.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useState } from "react";
import { expect } from "storybook/test";
import { webUIConfigQuery } from "../../api/queries";
import type { WebUIConfig } from "../../api/types";
import { basemapAttributionQuery } from "../../lib/attribution";
import { DataSources } from "./DataSources";

const BASEMAPS: WebUIConfig["basemaps"] = [
  { name: "Streets", styleUrl: "https://tiles.example.test/bright", darkCartography: false },
  { name: "Satellite", styleUrl: "https://imagery.example.test/aerial", darkCartography: true },
];

const CREDITS: Record<string, string> = {
  "https://tiles.example.test/bright": "© OpenStreetMap contributors",
  "https://imagery.example.test/aerial": "© Example Imagery",
};

function Seeded({ basemaps }: { basemaps: WebUIConfig["basemaps"] }): ReactNode {
  const [client] = useState(() => {
    const next = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
    });
    next.setQueryData(webUIConfigQuery().queryKey, {
      basemaps,
      sourceBaseUrls: {},
      identity: { email: "rider@example.test" },
    } as WebUIConfig);
    for (const basemap of basemaps) {
      next.setQueryData(
        basemapAttributionQuery(basemap.styleUrl).queryKey,
        CREDITS[basemap.styleUrl] ?? "",
      );
    }

    return next;
  });

  return (
    <QueryClientProvider client={client}>
      <div className="max-w-2xl p-4">
        <DataSources />
      </div>
    </QueryClientProvider>
  );
}

const meta = {
  title: "Settings/DataSources",
  component: DataSources,
} satisfies Meta<typeof DataSources>;

export default meta;

type Story = StoryObj<typeof meta>;

/** Two configured basemaps, plus the two credits every deployment owes. */
export const Default: Story = {
  render: () => <Seeded basemaps={BASEMAPS} />,
  play: async ({ canvas }) => {
    await expect(canvas.getByText(/© OpenStreetMap contributors/)).toBeVisible();
    await expect(canvas.getByText(/© Example Imagery/)).toBeVisible();
    await expect(canvas.getByText(/Surface data/)).toBeVisible();
    await expect(canvas.getByText(/Open-Meteo/)).toBeVisible();
  },
};

/**
 * A deployment on the single keyless default. The surface and weather credits
 * are owed whatever the operator configured, so they are still here.
 */
export const OneBasemap: Story = {
  render: () => <Seeded basemaps={[BASEMAPS[0] as WebUIConfig["basemaps"][number]]} />,
  play: async ({ canvas }) => {
    await expect(canvas.getByText(/© OpenStreetMap contributors/)).toBeVisible();
    await expect(canvas.queryByText(/© Example Imagery/)).not.toBeInTheDocument();
    await expect(canvas.getByText(/Surface data/)).toBeVisible();
  },
};
