/**
 * The card is the one place every credit this service owes is shown, so what is
 * asked here is that none of them can go missing.
 */

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

function show(
  basemaps: WebUIConfig["basemaps"],
  credits: Record<string, string> = {},
): QueryClient {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(webUIConfigQuery().queryKey, config(basemaps));
  for (const [styleUrl, credit] of Object.entries(credits)) {
    client.setQueryData(basemapAttributionQuery(styleUrl).queryKey, credit);
  }
  render(
    <QueryClientProvider client={client}>
      <DataSources />
    </QueryClientProvider>,
  );

  return client;
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
  /*
   * The two credits the service owes whatever an operator configured: neither
   * comes from a style document, so neither can be lost to a provider that
   * declares nothing.
   */
  it("always credits the surface and weather data", async () => {
    show([]);

    await waitFor(() => {
      expect(screen.getByText(/Surface data © OpenStreetMap contributors/)).toBeInTheDocument();
    });
    expect(screen.getByText(/Weather data by Open-Meteo/)).toBeInTheDocument();
  });

  /*
   * Every configured basemap is read, not only the one on screen: a reader may
   * switch to any of them, and this page does not know which is loaded.
   */
  it("credits every configured basemap's provider", async () => {
    show([STREETS, SATELLITE], {
      [STREETS.styleUrl]: "© Demo Cartography",
      [SATELLITE.styleUrl]: "© Demo Imagery",
    });

    await waitFor(() => {
      expect(screen.getByText(/© Demo Cartography/)).toBeInTheDocument();
    });
    expect(screen.getByText(/© Demo Imagery/)).toBeInTheDocument();
  });

  /*
   * Several entries usually come from one provider declaring one thing. The
   * obligation is to show the credit, not to show it once per picker entry.
   */
  it("says one provider's credit once, however many basemaps carry it", async () => {
    show([STREETS, SATELLITE], {
      [STREETS.styleUrl]: "© One Provider",
      [SATELLITE.styleUrl]: "© One Provider",
    });

    await waitFor(() => {
      expect(screen.getAllByText("© One Provider")).toHaveLength(1);
    });
  });

  /* A style that declares nothing contributes nothing, rather than an empty row. */
  it("leaves out a basemap whose style declares nothing", async () => {
    show([STREETS, SATELLITE], {
      [STREETS.styleUrl]: "© Demo Cartography",
      [SATELLITE.styleUrl]: "",
    });

    await waitFor(() => {
      expect(screen.getByText("© Demo Cartography")).toBeInTheDocument();
    });
    expect(screen.getByText("© Demo Cartography").textContent).toBe("© Demo Cartography");
  });
});
