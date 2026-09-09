/**
 * The ride map's own furniture, without a canvas: the expand toggle it adds
 * beside the zoom pair, which `ActivityPage.test.tsx` mocks away.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { webUIConfigQuery } from "../../api/queries";
import type { WebUIConfig } from "../../api/types";
import { ChromeMap } from "../../storybook/mapMock";
import { ActivityMap } from "./ActivityMap";

// The overlay draws the recorded track itself, which needs a real MapLibre
// instance this test never mounts; only the furniture around it is in question.
vi.mock("../routes/RouteOverlay", () => ({ RouteOverlay: () => null }));

const CONFIG: WebUIConfig = {
  basemaps: [
    { name: "Streets", styleUrl: "https://example.test/style.json", darkCartography: false },
  ],
  sourceBaseUrls: {},
  timezone: "Europe/Berlin",
  identity: { display: "rider@example.test", admin: false },
};

const WORLD = {
  id: 9,
  name: "Makuri Islands",
  mapUrl: "/v1/zwift/worlds/9/map",
  bounds: { north: -10.73746, west: 165.76591, south: -10.85234, east: 165.88222 },
};

function show(expanded: boolean, onExpandedChange = vi.fn(), world?: typeof WORLD) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(webUIConfigQuery().queryKey, CONFIG);

  render(
    <QueryClientProvider client={client}>
      <ChromeMap>
        <ActivityMap
          coordinates={[]}
          world={world ?? null}
          bounds={[8.4, 49, 8.6, 49.2]}
          profile={null}
          activeMetres={null}
          onActiveChange={() => {}}
          expanded={expanded}
          onExpandedChange={onExpandedChange}
        />
      </ChromeMap>
    </QueryClientProvider>,
  );

  return onExpandedChange;
}

describe("ActivityMap's expand toggle", () => {
  it("offers to expand when collapsed", () => {
    show(false);

    expect(screen.getByRole("button", { name: "Expand map" })).toBeInTheDocument();
  });

  it("offers to collapse when expanded, and reports a press", async () => {
    const user = userEvent.setup();
    const onExpandedChange = show(true);

    await user.click(screen.getByRole("button", { name: "Collapse map" }));

    expect(onExpandedChange).toHaveBeenCalledWith(false);
  });
});

// A ride in a virtual world is drawn over that world's own artwork: its
// coordinates are not the ground's, so no basemap could be true under them.
describe("ActivityMap in a virtual world", () => {
  it("draws the world's artwork instead of a basemap, and keeps the toggle", async () => {
    const onExpandedChange = show(false, vi.fn(), WORLD);

    const artwork = screen.getByRole("img", { name: "Map of Makuri Islands" });
    expect(artwork).toHaveAttribute("src", "/v1/zwift/worlds/9/map");
    expect(screen.queryByLabelText("Recorded track")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Expand map" }));
    expect(onExpandedChange).toHaveBeenCalledWith(true);
  });
});
