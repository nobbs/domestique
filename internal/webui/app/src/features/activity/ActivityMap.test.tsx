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
import { stubPendingFetch } from "../../test/network";
import { ActivityMap } from "./ActivityMap";

// The overlay draws the recorded track itself, and a world's own artwork is a
// source and a layer of its own — both need a real MapLibre instance this
// test never mounts. Only the furniture around them is in question.
vi.mock("../routes/RouteOverlay", () => ({ RouteOverlay: () => null }));
vi.mock("react-map-gl/maplibre", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react-map-gl/maplibre")>()),
  Source: () => null,
  Layer: () => null,
}));

// What the camera was actually asked to frame: `MapViewport`'s own effects
// need a live map instance this test never mounts, so the prop it would have
// acted on is captured here instead.
const framed = vi.hoisted(() => ({ bounds: null as unknown }));
vi.mock("../../components/map/MapViewport", () => ({
  MapViewport: (props: { bounds: unknown }) => {
    framed.bounds = props.bounds;

    return null;
  },
}));

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
  it("draws nothing outdoors until the basemap config has answered", () => {
    stubPendingFetch();
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
    });

    const { container } = render(
      <QueryClientProvider client={client}>
        <ChromeMap>
          <ActivityMap
            coordinates={[]}
            world={null}
            bounds={[8.4, 49, 8.6, 49.2]}
            profile={null}
            activeMetres={null}
            onActiveChange={() => {}}
            expanded={false}
            onExpandedChange={() => {}}
          />
        </ChromeMap>
      </QueryClientProvider>,
    );

    expect(container).toBeEmptyDOMElement();
  });

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
  it("labels the canvas for the world instead of a basemap, and keeps the toggle", async () => {
    const onExpandedChange = show(false, vi.fn(), WORLD);

    expect(
      screen.getByRole("region", { name: "Recorded track in Makuri Islands" }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Recorded track" })).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Expand map" }));
    expect(onExpandedChange).toHaveBeenCalledWith(true);
  });

  it("offers no geolocate button: a world has nothing real to find", () => {
    show(false, vi.fn(), WORLD);

    expect(screen.queryByRole("button", { name: "Find my location" })).not.toBeInTheDocument();
  });

  // The world's own bounds are the whole island, which is what places its
  // artwork — framing the camera to them too zoomed every ride out to the
  // island regardless of how short it was, however far it sat from centre.
  it("frames the camera to the ride's own track, not the whole world", () => {
    show(false, vi.fn(), WORLD);

    expect(framed.bounds).toEqual([8.4, 49, 8.6, 49.2]);
  });
});
