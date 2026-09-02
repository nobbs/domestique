/** The credits card. A story must not reach a tile provider, so the tile
 * credits are seeded rather than fetched. */

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

const CREDITS: Record<string, string[]> = {
  "https://tiles.example.test/bright": ["© Example Cartography"],
  "https://imagery.example.test/aerial": ["© Example Imagery"],
};

function Seeded({ basemaps }: { basemaps: WebUIConfig["basemaps"] }): ReactNode {
  const [client] = useState(() => {
    const next = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
    });
    next.setQueryData(webUIConfigQuery().queryKey, {
      basemaps,
      sourceBaseUrls: {},
      identity: { display: "rider@example.test" },
    } as WebUIConfig);
    for (const basemap of basemaps) {
      next.setQueryData(
        basemapAttributionQuery(basemap.styleUrl, basemap.styleUrlDark).queryKey,
        CREDITS[basemap.styleUrl] ?? [],
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

export const Default: Story = {
  render: () => <Seeded basemaps={BASEMAPS} />,
  play: async ({ canvas }) => {
    await expect(canvas.getByText(/© Example Cartography/)).toBeVisible();
    await expect(canvas.getByText(/© Example Imagery/)).toBeVisible();
    await expect(canvas.getByText(/Surface data/)).toBeVisible();
    await expect(canvas.getByText(/Open-Meteo/)).toBeVisible();
  },
};

export const OneBasemap: Story = {
  render: () => <Seeded basemaps={[BASEMAPS[0] as WebUIConfig["basemaps"][number]]} />,
  play: async ({ canvas }) => {
    await expect(canvas.getByText(/© Example Cartography/)).toBeVisible();
    await expect(canvas.queryByText(/© Example Imagery/)).not.toBeInTheDocument();
    await expect(canvas.getByText(/Surface data/)).toBeVisible();
  },
};
