import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { webUIConfigQuery } from "../../api/queries";
import type { WebUIConfig } from "../../api/types";
import { basemapAttributionQuery } from "../../lib/attribution";
import { DataSources } from "./DataSources";

function config(basemaps: WebUIConfig["basemaps"]): WebUIConfig {
  return { basemaps, sourceBaseUrls: {}, identity: { email: "rider@example.test" } } as WebUIConfig;
}

function show(basemaps: WebUIConfig["basemaps"], credits: Record<string, string[]> = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(webUIConfigQuery().queryKey, config(basemaps));
  for (const basemap of basemaps) {
    client.setQueryData(
      basemapAttributionQuery(basemap.styleUrl, basemap.styleUrlDark).queryKey,
      credits[basemap.styleUrl] ?? [],
    );
  }
  render(
    <QueryClientProvider client={client}>
      <DataSources />
    </QueryClientProvider>,
  );
}

const STREETS = {
  name: "Streets",
  styleUrl: "https://tiles.test/streets.json",
  darkCartography: false,
};
const SATELLITE = {
  name: "Satellite",
  styleUrl: "https://imagery.test/satellite.json",
  darkCartography: true,
};

describe("DataSources", () => {
  it("credits the surface and weather data whatever is configured", async () => {
    show([]);

    await waitFor(() => {
      expect(screen.getByText(/Surface data © OpenStreetMap contributors/)).toBeInTheDocument();
    });
    expect(screen.getByText(/Weather data by Open-Meteo/)).toBeInTheDocument();
  });

  it("credits every configured basemap's provider", async () => {
    show([STREETS, SATELLITE], {
      [STREETS.styleUrl]: ["© Demo Cartography"],
      [SATELLITE.styleUrl]: ["© Demo Imagery"],
    });

    await waitFor(() => {
      expect(screen.getByText(/© Demo Cartography/)).toBeInTheDocument();
    });
    expect(screen.getByText(/© Demo Imagery/)).toBeInTheDocument();
  });

  it("says one provider's credit once, however many basemaps carry it", async () => {
    show([STREETS, SATELLITE], {
      [STREETS.styleUrl]: ["© One Provider"],
      [SATELLITE.styleUrl]: ["© One Provider"],
    });

    await waitFor(() => {
      expect(screen.getAllByText("© One Provider")).toHaveLength(1);
    });
  });

  it("leaves out a basemap whose style declares nothing", async () => {
    show([STREETS, SATELLITE], {
      [STREETS.styleUrl]: ["© Demo Cartography"],
      [SATELLITE.styleUrl]: [],
    });

    await waitFor(() => {
      expect(screen.getByText("© Demo Cartography")).toBeInTheDocument();
    });
  });

  // This card is the only place any credit is shown, so a provider that cannot
  // be read must not take the others with it.
  it("keeps the rest when one basemap's credit could not be read", async () => {
    show([STREETS, SATELLITE], { [SATELLITE.styleUrl]: ["© Demo Imagery"] });

    await waitFor(() => {
      expect(screen.getByText(/© Demo Imagery/)).toBeInTheDocument();
    });
    expect(screen.getByText(/Surface data/)).toBeInTheDocument();
    expect(screen.getByText(/Open-Meteo/)).toBeInTheDocument();
  });
});
